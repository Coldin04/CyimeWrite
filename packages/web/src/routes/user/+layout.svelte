<script lang="ts">
	import { onDestroy } from 'svelte';
	import * as m from '$paraglide/messages';
	import { get } from 'svelte/store';
	import RouteAuthGuard from '$lib/components/auth/RouteAuthGuard.svelte';
	import SettingsShell from '$lib/components/settings/SettingsShell.svelte';
	import { realtimeConfig } from '$lib/stores/realtime';
	import House from '~icons/ph/house';
	import UserCircle from '~icons/ph/user-circle';
	import ShieldCheck from '~icons/ph/shield-check';
	import ImagesSquare from '~icons/ph/images-square';
	import LinkSimple from '~icons/ph/link-simple';
	import UsersThree from '~icons/ph/users-three';

	let { children } = $props();
	let realtimeConfigSignal = $state(get(realtimeConfig));
	const collaborationEnabled = $derived(realtimeConfigSignal.config?.collaborationEnabled ?? false);
	const unsubscribeRealtimeConfig = realtimeConfig.subscribe((state) => {
		realtimeConfigSignal = state;
	});

	onDestroy(() => {
		unsubscribeRealtimeConfig();
	});

	const allNavItems = [
		{ href: '/user', label: m.user_nav_overview(), icon: House },
		{ href: '/user/profile', label: m.user_nav_profile(), icon: UserCircle },
		{ href: '/user/security', label: m.user_nav_security(), icon: ShieldCheck },
		{ href: '/user/sharing', label: m.user_nav_sharing(), icon: UsersThree },
		{ href: '/user/image-beds', label: m.user_nav_image_beds(), icon: LinkSimple },
		{ href: '/user/media', label: m.user_nav_media_library(), icon: ImagesSquare }
	];
	const navItems = $derived(
		allNavItems.filter((item) => collaborationEnabled || item.href !== '/user/sharing')
	);
</script>

<RouteAuthGuard mode="required">
	<SettingsShell navItems={navItems}>
		{@render children()}
	</SettingsShell>
</RouteAuthGuard>
