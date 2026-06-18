<script lang="ts">
	import { onMount } from 'svelte';
	import data from '@emoji-mart/data';
	import { Picker } from 'emoji-mart';

	let { onSelect, onClose, anchorRect = null }: {
		onSelect: (emoji: string) => void;
		onClose: () => void;
		anchorRect?: DOMRect | null;
	} = $props();

	let container: HTMLDivElement;

	onMount(() => {
		const picker = new Picker({
			data,
			theme: 'dark',
			set: 'native',
			previewPosition: 'none',
			skinTonePosition: 'none',
			onEmojiSelect: (e: any) => {
				onSelect(e.native);
				onClose();
			},
		});
		container.appendChild(picker as unknown as Node);
	});
</script>

<div class="overlay" onclick={onClose}></div>
<div class="picker-wrap" bind:this={container} style={anchorRect
	? `position:fixed;bottom:${window.innerHeight - anchorRect.top + 8}px;left:${Math.min(anchorRect.left, window.innerWidth - 360)}px`
	: ''}></div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		z-index: 98;
	}
	.picker-wrap {
		position: absolute;
		bottom: calc(100% + 8px);
		left: 0;
		z-index: 99;
	}
	:global(em-emoji-picker) {
		--border-radius: 8px;
		--background-rgb: 34, 34, 40;
		--rgb-accent: 232, 84, 30;
		--rgb-background: 26, 26, 33;
		--rgb-input: 38, 38, 43;
		--color-border: var(--border);
		--color-border-over: #3a3a45;
		height: 380px;
	}
</style>
