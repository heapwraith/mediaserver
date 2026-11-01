# syntax=docker/dockerfile:1
# Optional way to run mediaserver (docs/adr/011-docker.md). The plain binary
# stays the main way to install it.

# --- 1. frontend (embedded into the Go binary) ------------------------------
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
RUN npm run build

# --- 2. backend, no CGO ----------------------------------------------------
FROM golang:1.27-alpine AS build
# go.mod pins the exact patch version; Go downloads it when the image is older.
ENV GOTOOLCHAIN=auto CGO_ENABLED=0
ARG VERSION=docker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/build ./web/build
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/mediaserver ./cmd/mediaserver

# --- 3. FFmpeg 9 built from the official source, with NVENC ----------------
# Distro packages are too old (Debian 7.x, Alpine 6.1) or lack NVENC (Alpine).
# NVENC/NVDEC load the driver at run time (nv-codec-headers), so no CUDA SDK here.
FROM debian:bookworm-slim AS ffmpeg
ARG FFMPEG_VERSION=9.0.2
ARG FFMPEG_SHA256=8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e
ARG NVCODEC_TAG=n13.0.19.1
ARG CHROMAPRINT_VERSION=1.6.2
ARG CHROMAPRINT_SHA256=d54ada62c16b2db396d3251ec54fc7a58909c163131a1cfcf7fa6407dbc14360
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl xz-utils git build-essential nasm pkg-config cmake \
        libx264-dev libx265-dev libvpx-dev libopus-dev libmp3lame-dev libdav1d-dev \
        libass-dev libwebp-dev libva-dev \
    && rm -rf /var/lib/apt/lists/*
RUN git clone --depth 1 --branch "${NVCODEC_TAG}" https://github.com/FFmpeg/nv-codec-headers.git /nvh \
    && make -C /nvh install
# Chromaprint (intro/credits detection, internal/intro uses `-f chromaprint`).
# Static, with KissFFT: Debian's libchromaprint depends on Debian's own FFmpeg.
WORKDIR /chromaprint
RUN curl -fsSL --retry 5 --retry-all-errors -o cp.tar.gz "https://github.com/acoustid/chromaprint/releases/download/v${CHROMAPRINT_VERSION}/chromaprint-${CHROMAPRINT_VERSION}.tar.gz" \
    && echo "${CHROMAPRINT_SHA256}  cp.tar.gz" | sha256sum -c - \
    && tar -xzf cp.tar.gz --strip-components=1 \
    && cmake -B build -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DFFT_LIB=kissfft \
        -DBUILD_TOOLS=OFF -DBUILD_TESTS=OFF -DCMAKE_INSTALL_PREFIX=/usr/local \
    && cmake --build build -j"$(nproc)" && cmake --install build
WORKDIR /ffmpeg
RUN curl -fsSL --retry 5 --retry-all-errors -o ffmpeg.tar.xz "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" \
    && echo "${FFMPEG_SHA256}  ffmpeg.tar.xz" | sha256sum -c - \
    && tar -xJf ffmpeg.tar.xz --strip-components=1
RUN ./configure --prefix=/opt/ffmpeg --enable-gpl --enable-version3 \
        --enable-libx264 --enable-libx265 --enable-libvpx --enable-libopus \
        --enable-libmp3lame --enable-libdav1d --enable-libass --enable-libwebp \
        --enable-vaapi --enable-nvenc --enable-nvdec --enable-ffnvcodec \
        --enable-chromaprint --extra-libs="-lstdc++ -lm" \
        --disable-doc --disable-ffplay --disable-debug \
    && make -j"$(nproc)" && make install

# --- 4. final image (glibc: required by the NVIDIA container runtime) -------
FROM debian:bookworm-slim
# Runtime libraries of the FFmpeg above (names are the Debian bookworm ones).
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata wget fontconfig \
        libx264-164 libx265-199 libvpx7 libopus0 libmp3lame0 libdav1d6 libass9 libwebp7 libwebpmux3 libva2 libva-drm2 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 1000 --no-create-home --shell /usr/sbin/nologin mediaserver \
    && mkdir -p /data /media \
    && chown mediaserver /data
COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /usr/local/bin/
COPY --from=build /out/mediaserver /usr/local/bin/mediaserver

# NVIDIA_*: only used when the container gets a GPU (docker-compose.nvidia.yml).
ENV HOST=0.0.0.0 PORT=8080 DATA_DIR=/data MEDIA_DIR=/media \
    NVIDIA_VISIBLE_DEVICES=all NVIDIA_DRIVER_CAPABILITIES=compute,video,utility
USER mediaserver
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${PORT}/api/health" >/dev/null || exit 1
ENTRYPOINT ["mediaserver"]
