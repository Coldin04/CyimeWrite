<script lang="ts">
	import { goto } from '$app/navigation';
	import { onDestroy } from 'svelte';
	import { get } from 'svelte/store';
	import { page } from '$app/stores';
	import * as m from '$paraglide/messages';
	import { toast } from 'svelte-sonner';
	import OverviewTab from '$lib/components/user/OverviewTab.svelte';
	import ProfileTab from '$lib/components/user/ProfileTab.svelte';
	import SecurityTab from '$lib/components/user/SecurityTab.svelte';
	import ApiTokensTab from '$lib/components/user/ApiTokensTab.svelte';
	import MediaTab from '$lib/components/user/MediaTab.svelte';
	import ImageBedsTab from '$lib/components/user/ImageBedsTab.svelte';
	import SharingTab from '$lib/components/user/SharingTab.svelte';
	import { realtimeConfig } from '$lib/stores/realtime';

	let tab = $derived($page.params.tab || 'overview');
	let realtimeConfigSignal = $state(get(realtimeConfig));
	const collaborationEnabled = $derived(realtimeConfigSignal.config?.collaborationEnabled ?? false);
	const unsubscribeRealtimeConfig = realtimeConfig.subscribe((state) => {
		realtimeConfigSignal = state;
	});

	onDestroy(() => {
		unsubscribeRealtimeConfig();
	});

	const titles: Record<string, any> = {
		get overview() { return m.user_nav_overview(); },
		get profile() { return m.user_nav_profile(); },
		get 'image-beds'() { return m.user_nav_image_beds(); },
		get security() { return m.user_security_title(); },
		get 'api-tokens'() { return m.user_api_tokens_title(); },
		get media() { return m.user_media_title(); },
		get sharing() { return m.user_nav_sharing(); }
	};

	const descriptions: Record<string, any> = {
		get overview() { return m.user_center_description(); },
		get profile() { return m.user_profile_description(); },
		get 'image-beds'() { return m.user_image_beds_description(); },
		get security() { return m.user_security_description(); },
		get 'api-tokens'() { return m.user_api_tokens_description(); },
		get media() { return m.user_media_description(); },
		get sharing() { return m.user_sharing_description(); }
	};

	$effect(() => {
		if (tab === 'sharing' && !realtimeConfigSignal.loading && !collaborationEnabled) {
			toast.error(m.workspace_shared_disabled());
			void goto('/user');
		}
	});
</script>

<svelte:head>
	<title>{titles[tab] || m.user_center_title()} - {m.page_title_user_center()}</title>
</svelte:head>

<section class="space-y-6">
	{#if tab !== 'image-beds'}
		<div>
			<h1 class="text-2xl font-bold text-zinc-900 dark:text-zinc-100">{titles[tab]}</h1>
			<p class="mt-1 text-sm text-zinc-600 dark:text-zinc-400">
				{descriptions[tab] || ''}
			</p>
		</div>
	{/if}

	{#if tab === 'overview'}
		<OverviewTab />
	{:else if tab === 'profile'}
		<ProfileTab />
	{:else if tab === 'image-beds'}
		<ImageBedsTab />
	{:else if tab === 'security'}
		<SecurityTab />
	{:else if tab === 'api-tokens'}
		<ApiTokensTab />
	{:else if tab === 'media'}
		<MediaTab />
	{:else if tab === 'sharing' && collaborationEnabled}
		<SharingTab />
	{/if}
</section>
