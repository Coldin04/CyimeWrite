<script lang="ts">
	import type { FileItem } from '$lib/api/workspace';
	import DocumentListItem from '$lib/components/workspace/DocumentListItem.svelte';

	let {
		item,
		selectedItems,
		bulkMode = false,
		onToggle,
		onRefresh,
		collaborationEnabled = false
	}: {
		item: FileItem;
		selectedItems: { [key: string]: boolean };
		bulkMode?: boolean;
		onToggle: (id: string) => void;
		onRefresh?: () => void;
		collaborationEnabled?: boolean;
	} = $props();
</script>

<DocumentListItem
	{item}
	{selectedItems}
	{bulkMode}
	{onToggle}
	{onRefresh}
	{collaborationEnabled}
	iconKind="table"
/>
