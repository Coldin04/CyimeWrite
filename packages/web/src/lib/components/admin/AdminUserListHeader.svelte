<script lang="ts">
	import * as m from '$paraglide/messages';
</script>

<div class="border-b border-zinc-200 dark:border-zinc-700">
	<div class="flex items-center justify-between px-4 py-2 text-sm font-semibold text-zinc-500 dark:text-zinc-400">
		<div class="min-w-0">
			<span>{m.common_user()}</span>
		</div>
		<div class="flex items-center justify-end gap-x-4 sm:gap-x-6">
			<div class="hidden w-28 text-right sm:block">{m.admin_users_used_documents()}</div>
			<div class="hidden w-28 text-right md:block">{m.admin_users_effective_quota()}</div>
			<div class="w-12 text-right">{m.admin_users_manage()}</div>
		</div>
	</div>
</div>
