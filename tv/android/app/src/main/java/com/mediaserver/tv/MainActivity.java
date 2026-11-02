package com.mediaserver.tv;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.pm.ApplicationInfo;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.view.Display;
import android.view.KeyEvent;
import android.view.WindowManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;

import java.net.Inet4Address;
import java.net.InetAddress;
import java.net.NetworkInterface;
import java.util.Collections;

/**
 * The Android TV app: a WebView that opens the connection page (assets, built
 * from web/src/shell) and then the app of the chosen server. The page talks to
 * it through window.MediaserverAndroid (web/src/lib/platform/bridge.ts).
 */
public class MainActivity extends Activity {
    private static final String SHELL = "file:///android_asset/index.html";

    private WebView web;
    /** Origin of the server opened with connect(); the only site the WebView may show. */
    private volatile String serverOrigin = "";

    @SuppressLint("SetJavaScriptEnabled")
    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        // A TV app: the screen saver must not start over a movie.
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        if ((getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0) {
            WebView.setWebContentsDebuggingEnabled(true);
        }

        web = new WebView(this);
        web.setBackgroundColor(0xFF0B0B0F);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true); // profile, settings and the saved server
        s.setMediaPlaybackRequiresUserGesture(false); // next episode, trailers
        web.addJavascriptInterface(new Bridge(), "MediaserverAndroid");
        web.setWebViewClient(new Client());
        web.setWebChromeClient(new WebChromeClient());
        setContentView(web);
        web.loadUrl(SHELL);
    }

    @Override
    protected void onDestroy() {
        web.destroy();
        super.onDestroy();
    }

    /**
     * Back, media and color keys go to the page as keydown events: the WebView
     * does not deliver them by itself (web/src/lib/platform/keys.ts maps them).
     */
    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        String[] key = pageKey(event.getKeyCode());
        if (key == null) return super.dispatchKeyEvent(event);
        boolean repeatable = event.getKeyCode() == KeyEvent.KEYCODE_MEDIA_FAST_FORWARD
                || event.getKeyCode() == KeyEvent.KEYCODE_MEDIA_REWIND;
        if (event.getAction() == KeyEvent.ACTION_DOWN && (event.getRepeatCount() == 0 || repeatable)) {
            web.evaluateJavascript("(function(k,c){var t=document.activeElement||document.body;"
                    + "var e=new KeyboardEvent('keydown',{key:k,bubbles:true,cancelable:true});"
                    + "Object.defineProperty(e,'keyCode',{get:function(){return c}});t.dispatchEvent(e);})('"
                    + key[0] + "'," + key[1] + ")", null);
        }
        return true;
    }

    /** DOM key name and the code Samsung/LG send for it, or null for keys the WebView handles. */
    private static String[] pageKey(int code) {
        switch (code) {
            case KeyEvent.KEYCODE_BACK: return new String[] {"GoBack", "10009"};
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE: return new String[] {"MediaPlayPause", "10252"};
            case KeyEvent.KEYCODE_MEDIA_PLAY: return new String[] {"MediaPlay", "415"};
            case KeyEvent.KEYCODE_MEDIA_PAUSE: return new String[] {"MediaPause", "19"};
            case KeyEvent.KEYCODE_MEDIA_STOP: return new String[] {"MediaStop", "413"};
            case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD: return new String[] {"MediaFastForward", "417"};
            case KeyEvent.KEYCODE_MEDIA_REWIND: return new String[] {"MediaRewind", "412"};
            case KeyEvent.KEYCODE_PROG_RED: return new String[] {"ColorF0Red", "403"};
            case KeyEvent.KEYCODE_PROG_GREEN: return new String[] {"ColorF1Green", "404"};
            case KeyEvent.KEYCODE_PROG_YELLOW: return new String[] {"ColorF2Yellow", "405"};
            case KeyEvent.KEYCODE_PROG_BLUE: return new String[] {"ColorF3Blue", "406"};
            default: return null;
        }
    }

    private static String originOf(String url) {
        Uri u = Uri.parse(url);
        if (u.getScheme() == null || u.getHost() == null) return "";
        return u.getScheme() + "://" + u.getHost() + (u.getPort() == -1 ? "" : ":" + u.getPort());
    }

    /** Called by the page (any thread): window.MediaserverAndroid. */
    private final class Bridge {
        @JavascriptInterface
        public void exit() {
            runOnUiThread(MainActivity.this::finish);
        }

        @JavascriptInterface
        public void openSetup() {
            runOnUiThread(() -> web.loadUrl(SHELL + "#setup"));
        }

        @JavascriptInterface
        public void connect(String url) {
            if (url == null || !url.startsWith("http://") && !url.startsWith("https://")) return;
            serverOrigin = originOf(url);
            runOnUiThread(() -> web.loadUrl(url));
        }

        /** IPv4 of the device on the local network, cable first ('' when offline). */
        @JavascriptInterface
        public String localIp() {
            String wireless = "";
            try {
                for (NetworkInterface ni : Collections.list(NetworkInterface.getNetworkInterfaces())) {
                    if (!ni.isUp() || ni.isLoopback()) continue;
                    for (InetAddress a : Collections.list(ni.getInetAddresses())) {
                        if (!(a instanceof Inet4Address) || !a.isSiteLocalAddress()) continue;
                        if (ni.getName().startsWith("eth")) return a.getHostAddress();
                        if (wireless.isEmpty()) wireless = a.getHostAddress();
                    }
                }
            } catch (Exception e) {
                return wireless;
            }
            return wireless;
        }

        /** The screen modes as JSON ('' below Android 6, which cannot switch them). */
        @JavascriptInterface
        public String displayModes() {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) return "";
            @SuppressWarnings("deprecation")
            Display display = getWindowManager().getDefaultDisplay();
            StringBuilder json = new StringBuilder("{\"current\":")
                    .append(display.getMode().getModeId()).append(",\"modes\":[");
            String separator = "";
            for (Display.Mode m : display.getSupportedModes()) {
                json.append(separator).append("{\"id\":").append(m.getModeId())
                        .append(",\"width\":").append(m.getPhysicalWidth())
                        .append(",\"height\":").append(m.getPhysicalHeight())
                        .append(",\"rate\":").append(m.getRefreshRate()).append('}');
                separator = ",";
            }
            return json.append("]}").toString();
        }

        /** Asks for a screen mode while the app is open; 0 goes back to the default. */
        @JavascriptInterface
        public void setDisplayMode(int id) {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) return;
            runOnUiThread(() -> {
                WindowManager.LayoutParams attrs = getWindow().getAttributes();
                attrs.preferredDisplayModeId = id;
                getWindow().setAttributes(attrs);
            });
        }
    }

    private final class Client extends WebViewClient {
        /** Only the connection page and the server's app: links elsewhere are ignored. */
        private boolean blocked(String url) {
            return !url.startsWith(SHELL) && !(serverOrigin.length() > 0 && originOf(url).equals(serverOrigin));
        }

        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
            return blocked(request.getUrl().toString());
        }

        @SuppressWarnings("deprecation")
        @Override
        public boolean shouldOverrideUrlLoading(WebView view, String url) {
            return blocked(url);
        }

        /** The server went away: back to the connection page (it shows the error). */
        @Override
        public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M && request.isForMainFrame()
                    && !request.getUrl().toString().startsWith(SHELL)) {
                view.loadUrl(SHELL);
            }
        }

        @SuppressWarnings("deprecation")
        @Override
        public void onReceivedError(WebView view, int code, String description, String url) {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M && url != null && !url.startsWith(SHELL)) {
                view.loadUrl(SHELL);
            }
        }
    }
}
