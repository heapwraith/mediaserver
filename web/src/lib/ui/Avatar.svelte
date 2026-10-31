<script lang="ts">
	/** Profile avatar: the initial on the profile color. `size` is in rem. */
	let { name, color, size = 8 }: { name: string; color: string; size?: number } = $props();

	const initial = $derived(([...name.trim()][0] ?? '?').toUpperCase());
</script>

<span
	class="avatar"
	style:width={`${size}rem`}
	style:height={`${size}rem`}
	style:font-size={`${size * 0.45}rem`}
	style:background={color}
	aria-hidden="true">{initial}</span
>

<style>
	.avatar {
		display: inline-flex;
		flex: none;
		align-items: center;
		justify-content: center;
		border-radius: 999px;
		color: rgba(255, 255, 255, 0.92);
		font-weight: 700;
		line-height: 1;
		user-select: none;
	}
</style>
