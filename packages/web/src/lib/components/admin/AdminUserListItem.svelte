<script lang="ts">
	import { goto } from '$app/navigation';
	import * as m from '$paraglide/messages';
	import UserAvatar from '$lib/components/common/UserAvatar.svelte';
	import CaretRight from '~icons/ph/caret-right';
	import type { AdminUserListItem } from '$lib/api/admin';

	let { item }: { item: AdminUserListItem } = $props();

	function openDetail() {
		void goto(`/admin/users/${item.id}`);
	}

	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			openDetail();
		}
	}

	function formatQuotaSummary(): string {
		if (item.unlimited) {
			return m.admin_quota_unlimited();
		}
		return item.effectiveDocumentQuota === null ? '-' : String(item.effectiveDocumentQuota);
	}
</script>

<div
	role="button"
	tabindex="0"
	class="group flex cursor-pointer items-center justify-between border-b border-zinc-200 px-4 py-3 transition-colors hover:bg-gradient-to-r hover:from-blue-50/50 hover:to-transparent dark:border-zinc-700 dark:hover:bg-none dark:hover:bg-zinc-800/60"
	onclick={openDetail}
	onkeydown={handleKeyDown}
>
	<div class="flex min-w-0 items-start gap-3 pr-4">
		<UserAvatar size={36} name={item.displayName} avatarUrl={item.avatarUrl} />
		<div class="min-w-0">
			<span class="block truncate font-normal text-zinc-800 dark:text-zinc-200">
				{item.displayName || m.user_common_default_name()}
			</span>
			<p class="mt-0.5 truncate text-xs text-zinc-500 dark:text-zinc-400">
				{item.email || m.user_common_no_email()}
			</p>
		</div>
	</div>

	<div class="flex shrink-0 items-center justify-end gap-x-4 sm:gap-x-6">
		<div class="hidden w-28 text-right text-sm text-zinc-600 dark:text-zinc-400 sm:block">
			{item.usedDocumentCount}
		</div>
		<div class="hidden w-28 text-right text-sm text-zinc-600 dark:text-zinc-400 md:block">
			{formatQuotaSummary()}
		</div>
		<div class="flex w-12 justify-end text-zinc-400 transition-colors group-hover:text-zinc-700 dark:group-hover:text-zinc-200">
			<CaretRight class="h-5 w-5" />
		</div>
	</div>
</div>

