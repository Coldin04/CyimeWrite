<script lang="ts">
	import { page } from '$app/stores';
	import TopBar from '$lib/components/workspace/TopBar.svelte';
	import UserAvatar from '$lib/components/common/UserAvatar.svelte';
	import { auth } from '$lib/stores/auth';
	import * as m from '$paraglide/messages';
	import CaretDown from '~icons/ph/caret-down';

	type NavItem = {
		href: string;
		label: string;
		icon: any;
	};

	let {
		navItems,
		children
	}: {
		navItems: NavItem[];
		children: import('svelte').Snippet;
	} = $props();

	let mobileNavOpen = $state(false);

	function isActive(pathname: string, href: string): boolean {
		if (href === '/user' || href === '/admin') return pathname === href;
		return pathname.startsWith(href);
	}

	$effect(() => {
		$page.url.pathname;
		mobileNavOpen = false;
	});
</script>

<TopBar />
<main class="grid min-h-[calc(100vh-4rem)] grid-cols-1 gap-4 px-4 py-4 sm:px-6 lg:grid-cols-[260px_minmax(0,1fr)] lg:gap-0 lg:px-0 lg:py-0">
	<aside class="rounded-xl border border-zinc-200 bg-white p-2 dark:border-zinc-700 dark:bg-zinc-900 lg:hidden">
		<button
			type="button"
			class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left"
			onclick={() => (mobileNavOpen = !mobileNavOpen)}
		>
			<div class="flex min-w-0 items-center gap-3">
				<UserAvatar size={40} name={$auth.user?.displayName} avatarUrl={$auth.user?.avatarUrl} />
				<div class="min-w-0">
					<p class="truncate text-sm font-semibold text-zinc-900 dark:text-zinc-100">
						{$auth.user?.displayName || m.user_common_default_name()}
					</p>
					<p class="truncate text-xs text-zinc-500 dark:text-zinc-400">
						{$auth.user?.email || m.user_common_no_email()}
					</p>
				</div>
			</div>
			<CaretDown class={`h-4 w-4 text-zinc-500 transition-transform ${mobileNavOpen ? 'rotate-180' : ''}`} />
		</button>
		{#if mobileNavOpen}
			<nav class="space-y-1 px-1 pb-1">
				{#each navItems as item (item.href)}
					<a
						href={item.href}
						class={`flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors ${
							isActive($page.url.pathname, item.href)
								? 'bg-sky-50 font-semibold text-cyan-900 dark:bg-cyan-900/50 dark:text-cyan-200'
								: 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
						}`}
					>
						<item.icon class="h-4 w-4 flex-shrink-0" />
						{item.label}
					</a>
				{/each}
			</nav>
		{/if}
	</aside>

	<aside class="hidden space-y-5 border-r border-zinc-200 bg-white px-5 py-6 dark:border-zinc-800 dark:bg-zinc-900 lg:block">
		<div class="flex items-center gap-3">
			<UserAvatar size={44} name={$auth.user?.displayName} avatarUrl={$auth.user?.avatarUrl} />
			<div class="min-w-0">
				<p class="truncate text-sm font-semibold text-zinc-900 dark:text-zinc-100">
					{$auth.user?.displayName || m.user_common_default_name()}
				</p>
				<p class="truncate text-xs text-zinc-500 dark:text-zinc-400">
					{$auth.user?.email || m.user_common_no_email()}
				</p>
			</div>
		</div>
		<nav class="space-y-1">
			{#each navItems as item (item.href)}
				<a
					href={item.href}
					class={`flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors ${
						isActive($page.url.pathname, item.href)
							? 'bg-sky-50 text-cyan-900 shadow-[inset_0_0_0_1px_rgba(8,145,178,0.10)] dark:bg-sky-900/40 dark:text-cyan-100'
							: 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
					}`}
				>
					<item.icon class="h-4 w-4 flex-shrink-0" />
					{item.label}
				</a>
			{/each}
		</nav>
	</aside>

	<section class="min-w-0 rounded-2xl border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900 sm:p-6 lg:min-h-[calc(100vh-4rem)] lg:rounded-none lg:border-0 lg:bg-transparent lg:p-8 xl:p-10">
		<div class="mx-auto w-full max-w-7xl">
			{@render children()}
		</div>
	</section>
</main>
