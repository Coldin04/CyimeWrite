<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { getLocale } from '$paraglide/runtime';
	import { auth } from '$lib/stores/auth';
	import { clickOutside } from '$lib/actions/clickOutside';
	import { portal } from '$lib/actions/portal';
	import {
		acceptDocumentInvite,
		clearNotifications,
		declineDocumentInvite,
		listNotifications
	} from '$lib/api/workspace';
	import * as m from '$paraglide/messages';
	import Bell from '~icons/ph/bell';
	import CaretRight from '~icons/ph/caret-right';
	import ClockCounterClockwise from '~icons/ph/clock-counter-clockwise';
	import SignOut from '~icons/ph/sign-out';
	import ShieldCheck from '~icons/ph/shield-check';
	import Trash from '~icons/ph/trash';
	import X from '~icons/ph/x';
	import UserAvatar from '$lib/components/common/UserAvatar.svelte';
	import type { NotificationItem } from '$lib/api/workspace';

	interface Props {
		profileHref?: string;
		notificationHref?: string;
		trashHref?: string;
		showTrash?: boolean;
	}

	let {
		profileHref = '/user',
		notificationHref = '/workspace',
		trashHref = '/workspace/trash',
		showTrash = true
	}: Props = $props();

	let showMenu = $state(false);
	let showNotificationDrawer = $state(false);
	let unreadCount = $state(0);
	let isLoadingNotifications = $state(false);
	let isLoadingNotificationList = $state(false);
	let notificationError = $state('');
	let notifications = $state<NotificationItem[]>([]);
	let processingInviteId = $state('');
	let isClearingNotifications = $state(false);

	function toggleUserMenu() {
		showMenu = !showMenu;
		if (showMenu) {
			void refreshUnreadCount();
		}
	}

	function closeUserMenu() {
		showMenu = false;
	}

	function closeNotificationDrawer() {
		showNotificationDrawer = false;
	}

	function handleLogout() {
		auth.logout();
		showMenu = false;
	}

	async function refreshUnreadCount() {
		if (isLoadingNotifications) return;
		isLoadingNotifications = true;
		try {
			const data = await listNotifications({ unread: true, limit: 1, offset: 0 });
			unreadCount = data.unreadCount || 0;
		} catch (error) {
			console.error('Failed to load notifications:', error);
		} finally {
			isLoadingNotifications = false;
		}
	}

	async function refreshNotificationList() {
		if (isLoadingNotificationList) return;
		isLoadingNotificationList = true;
		notificationError = '';
		try {
			const data = await listNotifications({ unread: true, limit: 20, offset: 0 });
			notifications = data.items || [];
			unreadCount = data.unreadCount || 0;
		} catch (error) {
			notificationError =
				error instanceof Error ? error.message : m.workspace_notification_load_failed();
		} finally {
			isLoadingNotificationList = false;
		}
	}

	async function openNotificationDrawer() {
		showNotificationDrawer = true;
		await refreshNotificationList();
	}

	function getInviteData(item: NotificationItem): {
		inviteId: string;
		documentTitle: string;
		inviterDisplayName: string;
		role: string;
	} {
		const raw = item.data as Record<string, unknown> | null;
		if (!raw) {
			return {
				inviteId: '',
				documentTitle: m.workspace_notification_untitled_document(),
				inviterDisplayName: m.workspace_notification_system_message(),
				role: ''
			};
		}

		return {
			inviteId: String(raw.inviteId || ''),
			documentTitle: String(raw.documentTitle || m.workspace_notification_untitled_document()),
			inviterDisplayName: String(raw.inviterDisplayName || m.workspace_notification_system_message()),
			role: String(raw.role || '')
		};
	}

	function roleLabel(role: string): string {
		if (role === 'viewer') return m.workspace_shared_role_viewer();
		if (role === 'editor') return m.workspace_shared_role_editor();
		if (role === 'collaborator') return m.workspace_shared_role_collaborator();
		return role || m.workspace_notification_role_member();
	}

	function formatTime(ts: string): string {
		try {
			return new Intl.DateTimeFormat(getLocale(), {
				month: '2-digit',
				day: '2-digit',
				hour: '2-digit',
				minute: '2-digit'
			}).format(new Date(ts));
		} catch {
			return ts;
		}
	}

	async function handleInviteAction(inviteId: string, action: 'accept' | 'decline') {
		if (!inviteId || processingInviteId) return;

		processingInviteId = inviteId;
		try {
			if (action === 'accept') {
				await acceptDocumentInvite(inviteId);
				window.dispatchEvent(new CustomEvent('workspace:shared-documents-changed'));
				toast.success(m.workspace_notification_invite_accepted());
			} else {
				await declineDocumentInvite(inviteId);
				toast.success(m.workspace_notification_invite_declined());
			}
			await refreshNotificationList();
		} catch (error) {
			toast.error(
				error instanceof Error ? error.message : m.workspace_notification_invite_action_failed()
			);
		} finally {
			processingInviteId = '';
		}
	}

	async function handleClearNotifications() {
		if (isClearingNotifications) return;
		isClearingNotifications = true;
		try {
			const result = await clearNotifications();
			notifications = [];
			unreadCount = 0;
			toast.success(
				result.clearedCount > 0
					? m.workspace_notification_cleared_count({ count: result.clearedCount })
					: m.workspace_notification_cleared()
			);
		} catch (error) {
			toast.error(
				error instanceof Error ? error.message : m.workspace_notification_clear_failed()
			);
		} finally {
			isClearingNotifications = false;
		}
	}

	onMount(() => {
		if (window.location.pathname.startsWith('/workspace')) {
			void refreshUnreadCount();
		}
	});

	afterNavigate(({ to }) => {
		if (to?.url.pathname.startsWith('/workspace')) {
			void refreshUnreadCount();
		}
		closeUserMenu();
		closeNotificationDrawer();
	});

	$effect(() => {
		if (!showNotificationDrawer) return;
		const previousOverflow = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		return () => {
			document.body.style.overflow = previousOverflow;
		};
	});
</script>

<div
	class="relative"
	use:clickOutside={{
		enabled: showMenu,
		handler: closeUserMenu
	}}
>
	<button
		type="button"
		onclick={toggleUserMenu}
		class="relative flex h-9 w-9 items-center justify-center rounded-full border border-zinc-200 p-0 text-left transition-colors hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800 sm:h-auto sm:w-auto sm:justify-start sm:gap-2 sm:px-2 sm:py-1"
	>
		<UserAvatar size={28} name={$auth.user?.displayName} avatarUrl={$auth.user?.avatarUrl} />
		<div class="hidden min-w-0 sm:block">
			<p class="truncate text-xs font-semibold leading-tight text-zinc-800 dark:text-zinc-100">
				{$auth.user?.displayName || m.common_user()}
			</p>
			<p class="truncate text-[11px] leading-tight text-zinc-500 dark:text-zinc-400">
				{$auth.user?.email || m.user_common_no_email()}
			</p>
		</div>
		{#if unreadCount > 0}
			<span
				class="absolute right-0.5 top-0.5 h-3 w-3 shrink-0 aspect-square rounded-full bg-rose-500 ring-2 ring-white dark:ring-zinc-900 sm:right-1 sm:top-1"
			></span>
		{/if}
	</button>

	{#if showMenu}
		<div
			class="absolute top-full right-0 z-10 mt-2 w-56 origin-top-right rounded-md bg-white py-1 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none dark:bg-zinc-800 dark:ring-zinc-700"
		>
			<a
				href={profileHref}
				onclick={closeUserMenu}
				class="flex items-center justify-between gap-3 px-4 py-3 text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-700"
			>
				<span class="flex min-w-0 items-center gap-3">
					<UserAvatar size={36} name={$auth.user?.displayName} avatarUrl={$auth.user?.avatarUrl} />
					<span class="min-w-0">
						<span class="truncate text-sm font-semibold leading-tight">
							{$auth.user?.displayName || m.common_user()}
						</span>
						<span class="truncate text-xs leading-tight text-zinc-500 dark:text-zinc-400">
							{$auth.user?.email || m.user_common_no_email()}
						</span>
					</span>
				</span>
				<CaretRight class="h-3.5 w-3.5 shrink-0 text-zinc-400" />
			</a>

			<button
				type="button"
				onclick={() => {
					void openNotificationDrawer();
					closeUserMenu();
				}}
				class="flex w-full items-center justify-between gap-2 px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-700"
			>
				<span class="flex items-center gap-2">
					<Bell class="h-4 w-4" />
					<span>{m.workspace_notification_menu_label()}</span>
				</span>
				{#if unreadCount > 0}
					<span class="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-rose-500 px-1.5 text-[10px] font-semibold text-white">
						{unreadCount > 99 ? '99+' : unreadCount}
					</span>
				{/if}
				</button>

			{#if $auth.user?.adminAccess?.hasAccess}
				<a
					href="/admin"
					onclick={closeUserMenu}
					class="flex items-center gap-2 px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-700"
				>
					<ShieldCheck class="h-4 w-4" />
					<span>{m.admin_menu_entry()}</span>
				</a>
			{/if}

			{#if showTrash}
				<a
					href={trashHref}
					onclick={closeUserMenu}
					class="flex items-center gap-2 px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-700"
				>
					<Trash class="h-4 w-4" />
					<span>{m.topbar_trash()}</span>
				</a>
			{/if}

			<div class="my-1 h-px bg-zinc-200 dark:bg-zinc-700"></div>
			<button
				type="button"
				onclick={handleLogout}
				class="flex w-full items-center gap-2 px-4 py-2 text-left text-sm text-red-600 hover:bg-zinc-100 dark:text-red-400 dark:hover:bg-zinc-700"
			>
				<SignOut class="h-4 w-4" />
				<span>{m.topbar_logout()}</span>
			</button>
		</div>
	{/if}
</div>

{#if showNotificationDrawer}
	<div use:portal class="fixed inset-0 z-[110]">
		<button
			type="button"
			class="absolute inset-0 bg-black/20 backdrop-blur-[1px]"
			aria-label={m.workspace_notification_close_drawer()}
			onclick={closeNotificationDrawer}
		></button>

		<div
			class="absolute right-0 top-0 flex h-full w-screen flex-col border-l border-zinc-200 bg-white shadow-2xl dark:border-zinc-700 dark:bg-zinc-900 sm:w-[min(92vw,420px)]"
			role="dialog"
			aria-modal="true"
			aria-label={m.workspace_notification_drawer_aria_label()}
		>
			<div class="flex items-center justify-between border-b border-zinc-200 px-4 py-3 dark:border-zinc-700">
				<div>
					<p class="text-base font-semibold text-zinc-900 dark:text-zinc-100">{m.workspace_notification_title()}</p>
					<p class="text-xs text-zinc-500 dark:text-zinc-400">{m.workspace_notification_subtitle()}</p>
				</div>
				<div class="flex items-center gap-1">
					{#if notifications.length > 0}
						<button
							type="button"
							class="inline-flex h-8 items-center justify-center rounded-full px-3 text-xs font-medium text-zinc-500 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
							aria-label={m.workspace_notification_clear_action()}
							disabled={isClearingNotifications}
							onclick={() => {
								void handleClearNotifications();
							}}
						>
							{isClearingNotifications
								? m.workspace_notification_clearing_action()
								: m.workspace_notification_clear_action()}
						</button>
					{/if}
					<button
						type="button"
						class="inline-flex h-8 w-8 items-center justify-center rounded-full text-zinc-500 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
						aria-label={m.workspace_notification_refresh_action()}
						onclick={() => {
							void refreshNotificationList();
						}}
					>
						<ClockCounterClockwise class="h-4 w-4" />
					</button>
					<button
						type="button"
						class="inline-flex h-8 w-8 items-center justify-center rounded-full text-zinc-500 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
						aria-label={m.workspace_notification_close_drawer()}
						onclick={closeNotificationDrawer}
					>
						<X class="h-4 w-4" />
					</button>
				</div>
			</div>

			<div class="min-h-0 flex-1 overflow-y-auto px-3 py-3">
				{#if isLoadingNotificationList}
					<p class="px-2 py-6 text-center text-sm text-zinc-500 dark:text-zinc-400">{m.workspace_notification_loading()}</p>
				{:else if notificationError}
					<p class="px-2 py-6 text-center text-sm text-rose-600 dark:text-rose-400">{notificationError}</p>
				{:else if notifications.length === 0}
					<div class="px-2 py-10 text-center">
						<div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-zinc-100 dark:bg-zinc-800">
							<Bell class="h-5 w-5 text-zinc-400 dark:text-zinc-500" />
						</div>
						<p class="mt-3 text-sm font-medium text-zinc-700 dark:text-zinc-200">{m.workspace_notification_empty_title()}</p>
						<p class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{m.workspace_notification_empty_description()}</p>
					</div>
				{:else}
					<div class="space-y-2">
						{#each notifications as item}
							{@const invite = getInviteData(item)}
							<div
								class="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-3 dark:border-zinc-700 dark:bg-zinc-800/60"
							>
								<div class="flex items-start justify-between gap-3">
									<div class="min-w-0">
										<p class="truncate text-sm font-semibold text-zinc-900 dark:text-zinc-100">
											{m.workspace_notification_invite_title({ inviter: invite.inviterDisplayName })}
										</p>
										<p class="mt-1 line-clamp-2 text-sm text-zinc-600 dark:text-zinc-300">
											《{invite.documentTitle}》
										</p>
										<p class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
											{m.workspace_notification_invite_meta({
												role: roleLabel(invite.role),
												time: formatTime(item.createdAt)
											})}
										</p>
									</div>
									{#if !item.readAt}
										<span class="mt-1 h-2.5 w-2.5 shrink-0 rounded-full bg-rose-500"></span>
									{/if}
								</div>
								{#if invite.inviteId}
									<div class="mt-3 flex items-center gap-2">
										<button
											type="button"
											class="inline-flex flex-1 items-center justify-center rounded-lg bg-zinc-900 px-3 py-2 text-sm font-medium text-white transition hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
											disabled={processingInviteId === invite.inviteId}
											onclick={() => {
												void handleInviteAction(invite.inviteId, 'accept');
											}}
										>
											{processingInviteId === invite.inviteId
												? m.workspace_notification_processing_action()
												: m.workspace_notification_accept_action()}
										</button>
										<button
											type="button"
											class="inline-flex items-center justify-center rounded-lg border border-zinc-200 px-3 py-2 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:cursor-not-allowed disabled:opacity-60 dark:border-zinc-600 dark:text-zinc-300 dark:hover:bg-zinc-800"
											disabled={processingInviteId === invite.inviteId}
											onclick={() => {
												void handleInviteAction(invite.inviteId, 'decline');
											}}
										>
											{m.workspace_notification_decline_action()}
										</button>
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
