<script lang="ts">
	import { browser } from '$app/environment';
	import { tick } from 'svelte';
	import { portal } from '$lib/actions/portal';
	import type { DocumentImageTargetOption } from '$lib/components/editor/documentImageTargets';
	import {
		updateDocumentExcerpt,
		updateDocumentPublicAccess,
		updateDocumentTitle
	} from '$lib/api/workspace';
	import DocumentCollaborationSettings from '$lib/components/editor/DocumentCollaborationSettings.svelte';
	import { toast } from 'svelte-sonner';
	import * as m from '$paraglide/messages';
	import FileText from '~icons/ph/file-text';
	import ImageSquare from '~icons/ph/image-square';
	import LockKey from '~icons/ph/lock-key';
	import Copy from '~icons/ph/copy';
	import Check from '~icons/ph/check';
	import SlidersHorizontal from '~icons/ph/sliders-horizontal';
	import X from '~icons/ph/x';

	type DesignSectionId = 'basic' | 'permissions' | 'image';

	type Props = {
		documentId: string;
		documentTitle?: string;
		documentManualExcerpt?: string;
		documentType?: string;
		currentTargetId: string;
		options: DocumentImageTargetOption[];
		canEditBasic?: boolean;
		canManageMembers?: boolean;
		canEditImageSettings?: boolean;
		canManagePublic?: boolean;
		publicAccess?: 'private' | 'authenticated' | 'public' | string;
		publicUrl?: string;
		isUpdating?: boolean;
		trigger?: 'icon' | 'none';
		initialOpen?: boolean;
		onSelect: (targetId: string) => void | Promise<unknown>;
		onTitleChange?: (title: string) => void;
		onManualExcerptChange?: (excerpt: string) => void;
		onPublicAccessChange?: (publicAccess: string, publicUrl: string) => void;
		onOpenChange?: (open: boolean) => void;
	};

	type DesignSection = {
		id: DesignSectionId;
		icon: typeof FileText;
	};

	const sections: DesignSection[] = [
		{ id: 'basic', icon: FileText },
		{ id: 'permissions', icon: LockKey },
		{ id: 'image', icon: ImageSquare }
	];

	const sectionLabelMap: Record<DesignSectionId, () => string> = {
		basic: m.editor_document_settings_basic_title,
		permissions: m.editor_document_settings_permissions_title,
		image: m.editor_document_settings_image_title
	};

	let {
		documentId,
		documentTitle = '',
		documentManualExcerpt = '',
		documentType = 'rich_text',
		currentTargetId,
		options,
		canEditBasic = true,
		canManageMembers = false,
		canEditImageSettings = true,
		canManagePublic = false,
		publicAccess = 'private',
		publicUrl = '',
		isUpdating = false,
		trigger = 'icon',
		initialOpen = false,
		onSelect,
		onTitleChange,
		onManualExcerptChange,
		onPublicAccessChange,
		onOpenChange
	}: Props = $props();

	const visibleSections = $derived(
		sections.filter((section) => {
			switch (section.id) {
				case 'basic':
					return canEditBasic;
				case 'permissions':
					return canManageMembers || canManagePublic;
				case 'image':
					return canEditImageSettings;
				default:
					return false;
			}
		})
	);

	let open = $state(false);
	let activeSection = $state<DesignSectionId>('basic');
	let isEditingTitle = $state(false);
	let draftTitle = $state('');
	let draftExcerpt = $state('');
	let isSavingExcerpt = $state(false);
	let isSavingPublicAccess = $state(false);
	let copiedPublicURL = $state(false);
	let titleInput: HTMLInputElement | null = $state(null);
	let contentArea: HTMLElement | null = $state(null);
	let basicSection: HTMLElement | null = $state(null);
	let permissionsSection: HTMLElement | null = $state(null);
	let imageSection: HTMLElement | null = $state(null);
	let sectionObserver: IntersectionObserver | null = null;

	$effect(() => {
		if (initialOpen) {
			open = true;
		}
	});

	$effect(() => {
		if (!open || !isSavingPublicAccess) {
			draftPublicAccess = publicAccess;
		}
	});

	function getSectionElement(id: DesignSectionId): HTMLElement | null {
		switch (id) {
			case 'basic':
				return basicSection;
			case 'permissions':
				return permissionsSection;
			case 'image':
				return imageSection;
			default:
				return null;
		}
	}
	let draftPublicAccess = $state<'private' | 'authenticated' | 'public' | string>('private');

	$effect(() => {
		if (!browser) return;
		document.body.style.overflow = open ? 'hidden' : '';
		return () => {
			document.body.style.overflow = '';
		};
	});

	$effect(() => {
		if (!isEditingTitle) {
			draftTitle = documentTitle;
		}
	});

	$effect(() => {
		if (!open || !isSavingExcerpt) {
			draftExcerpt = documentManualExcerpt;
		}
	});

	$effect(() => {
		if (!open) {
			activeSection = (visibleSections[0]?.id ?? 'basic') as DesignSectionId;
			isEditingTitle = false;
		}
	});

	$effect(() => {
		if (!browser || !open || !contentArea) return;

		sectionObserver?.disconnect();
		sectionObserver = new IntersectionObserver(
			(entries) => {
				const visibleEntry = entries
					.filter((entry) => entry.isIntersecting)
					.sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0];

				const id = visibleEntry?.target.getAttribute('data-section-id') as DesignSectionId | null;
				if (id) {
					activeSection = id;
				}
			},
			{
				root: contentArea,
				rootMargin: '-12% 0px -55% 0px',
				threshold: [0.2, 0.45, 0.7]
			}
		);

		for (const section of visibleSections) {
			const el = getSectionElement(section.id);
			if (el) {
				sectionObserver.observe(el);
			}
		}

		return () => {
			sectionObserver?.disconnect();
			sectionObserver = null;
		};
	});

	function setOpen(nextOpen: boolean) {
		open = nextOpen;
		onOpenChange?.(nextOpen);
	}

	function closeDialog() {
		setOpen(false);
	}

	function scrollToSection(id: DesignSectionId) {
		activeSection = id;
		const el = getSectionElement(id);
		el?.scrollIntoView({
			behavior: 'smooth',
			block: 'start'
		});
	}

	function handleSelect(targetId: string) {
		if (targetId !== currentTargetId && !isUpdating) {
			void onSelect(targetId);
		}
	}

	function getDocumentTypeLabel(type: string) {
		if (type === 'table') return m.editor_document_settings_doc_type_table();
		if (type === 'rich_text') return m.editor_document_settings_doc_type_rich_text();
		return m.editor_document_settings_doc_type_default();
	}

	async function startEditingTitle() {
		if (!canEditBasic) return;
		draftTitle = documentTitle;
		isEditingTitle = true;
		await tick();
		titleInput?.focus();
	}

	function cancelEditingTitle() {
		isEditingTitle = false;
		draftTitle = documentTitle;
	}

	async function saveTitle() {
		if (!canEditBasic) return;
		const nextTitle = draftTitle.trim();
		if (!nextTitle || nextTitle === documentTitle) {
			isEditingTitle = false;
			draftTitle = documentTitle;
			return;
		}

		try {
			await updateDocumentTitle(documentId, nextTitle);
			onTitleChange?.(nextTitle);
			toast.success(m.editor_topbar_title_updated());
			isEditingTitle = false;
		} catch (error) {
			console.error('Failed to update title:', error);
			toast.error(m.editor_topbar_title_update_failed());
		}
	}

	async function saveExcerpt() {
		if (!canEditBasic) return;
		if (isSavingExcerpt) return;

		const nextExcerpt = draftExcerpt.trim();
		const currentExcerpt = documentManualExcerpt.trim();
		if (nextExcerpt === currentExcerpt) {
			draftExcerpt = documentManualExcerpt;
			return;
		}

		isSavingExcerpt = true;
		try {
			const response = await updateDocumentExcerpt(documentId, nextExcerpt);
			onManualExcerptChange?.(response.manualExcerpt);
			draftExcerpt = response.manualExcerpt;
			toast.success(m.editor_document_settings_description_updated());
		} catch (error) {
			console.error('Failed to update excerpt:', error);
			toast.error(
				error instanceof Error && error.message.trim() !== ''
					? error.message
					: m.editor_document_settings_description_update_failed()
			);
		} finally {
			isSavingExcerpt = false;
		}
	}

	function resolvePublicAccessURL() {
		const current = publicUrl.trim();
		const fallbackPath = `/view/documents/${documentId}`;
		const target = current !== '' ? current : fallbackPath;
		try {
			return new URL(target, window.location.origin).toString();
		} catch {
			return target;
		}
	}

	async function savePublicAccess() {
		if (!canManagePublic || isSavingPublicAccess) return;
		if (draftPublicAccess === publicAccess) return;
		isSavingPublicAccess = true;
		try {
			const response = await updateDocumentPublicAccess(documentId, draftPublicAccess);
			draftPublicAccess = response.publicAccess;
			onPublicAccessChange?.(response.publicAccess, response.publicUrl);
			toast.success(
				response.publicAccess === 'private'
					? m.editor_document_settings_public_disabled()
					: m.editor_document_settings_public_enabled()
			);
		} catch (error) {
			toast.error(
				error instanceof Error && error.message.trim() !== ''
					? error.message
					: m.editor_document_settings_public_update_failed()
			);
		} finally {
			isSavingPublicAccess = false;
		}
	}

	async function handlePublicAccessSelect(event: Event) {
		const nextValue = (event.currentTarget as HTMLSelectElement).value;
		draftPublicAccess = nextValue;
		await savePublicAccess();
	}

	async function copyPublicURL() {
		const targetURL = resolvePublicAccessURL();
		if (!targetURL) return;
		try {
			await navigator.clipboard.writeText(targetURL);
			copiedPublicURL = true;
			setTimeout(() => {
				copiedPublicURL = false;
			}, 1200);
			toast.success(m.editor_document_settings_public_copy_success());
		} catch {
			toast.error(m.editor_document_settings_public_copy_failed());
		}
	}

	function handleTitleKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter') {
			void saveTitle();
		} else if (event.key === 'Escape') {
			cancelEditingTitle();
		}
	}

	function getSectionLabel(id: DesignSectionId) {
		return sectionLabelMap[id]();
	}
</script>

{#if trigger !== 'none'}
	<button
		type="button"
		class="grid h-8 w-8 shrink-0 place-content-center rounded-full text-zinc-500 transition-colors hover:bg-black/10 hover:text-zinc-800 disabled:opacity-50 dark:text-zinc-400 dark:hover:bg-white/10 dark:hover:text-zinc-200"
		title={m.editor_topbar_image_target_settings()}
		aria-label={m.editor_topbar_image_target_settings()}
		disabled={isUpdating || visibleSections.length === 0}
		onclick={() => setOpen(true)}
	>
		<SlidersHorizontal class="h-5 w-5" />
	</button>
{/if}

{#if open}
	<div
		use:portal
		class="fixed inset-0 z-[120] min-h-dvh w-screen bg-black/45"
		role="presentation"
		onclick={closeDialog}
	>
		<div class="flex min-h-dvh w-full items-center justify-center p-0 sm:p-4">
			<div
				class="flex h-dvh w-full flex-col overflow-hidden bg-white shadow-2xl dark:bg-zinc-950 sm:h-[88vh] sm:max-h-[820px] sm:max-w-[960px] sm:rounded-xl sm:border sm:border-zinc-200 dark:sm:border-zinc-800"
				role="dialog"
				aria-modal="true"
				aria-label={m.editor_image_target_menu_title()}
				tabindex="-1"
				onclick={(event) => event.stopPropagation()}
				onkeydown={(event) => {
					if (event.key === 'Escape') closeDialog();
				}}
			>
				<header class="flex h-16 items-center justify-between gap-4 border-b border-zinc-200 px-5 dark:border-zinc-800 sm:px-6">
					<div class="flex min-w-0 items-center gap-3">
						<FileText class="h-5 w-5 shrink-0 text-zinc-400" />
						<div class="min-w-0 flex flex-col gap-0.5">
							<h2 class="truncate rounded bg-transparent px-2 text-sm leading-5 text-zinc-900 dark:text-zinc-100">
								{documentTitle || m.editor_document_settings_untitled()}
							</h2>
							<p class="px-2 text-xs leading-4 text-zinc-500 dark:text-zinc-400">
								{getDocumentTypeLabel(documentType)}
							</p>
						</div>
					</div>

					<button
						type="button"
						class="rounded-full p-2 text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
						onclick={closeDialog}
					>
						<X class="h-5 w-5" />
					</button>
				</header>

				<div class="flex min-h-0 flex-1 flex-col md:flex-row">
					<aside class="border-b border-zinc-200 bg-zinc-50/70 dark:border-zinc-800 dark:bg-zinc-900/40 md:w-56 md:shrink-0 md:border-b-0 md:border-r">
						<nav class="flex gap-1 overflow-x-auto p-3 md:h-full md:flex-col md:overflow-y-auto md:p-4">
							{#each visibleSections as section (section.id)}
								<button
									type="button"
									class={`inline-flex shrink-0 items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition md:w-full ${
										activeSection === section.id
											? 'bg-white text-zinc-900 shadow-sm dark:bg-zinc-950 dark:text-zinc-100'
											: 'text-zinc-500 hover:bg-white/70 dark:text-zinc-400 dark:hover:bg-zinc-950/70'
									}`}
									onclick={() => scrollToSection(section.id)}
								>
									<section.icon class="h-4 w-4" />
									<span>{getSectionLabel(section.id)}</span>
								</button>
							{/each}
						</nav>
					</aside>

					<section bind:this={contentArea} class="min-h-0 flex-1 overflow-y-auto px-5 py-4 sm:px-6 sm:py-5">
						<div class="max-w-2xl space-y-10 pb-12">
							{#if canEditBasic}
								<section
									bind:this={basicSection}
									data-section-id="basic"
									class="scroll-mt-6 space-y-6"
								>
								<div>
									<h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
										{m.editor_document_settings_basic_title()}
									</h3>
									<p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
										{m.editor_document_settings_basic_description()}
									</p>
								</div>

									<div class="space-y-3">
									<label for="document-title-input" class="text-xs font-medium text-zinc-500 dark:text-zinc-400">
										{m.editor_document_settings_title_label()}
									</label>
									<div class="flex flex-col gap-3 sm:flex-row sm:items-center">
										<input
											bind:this={titleInput}
											id="document-title-input"
											type="text"
											value={isEditingTitle ? draftTitle : documentTitle}
											onfocus={startEditingTitle}
											oninput={(event) => (draftTitle = event.currentTarget.value)}
											onkeydown={handleTitleKeydown}
											class="min-w-0 flex-1 rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 outline-none focus:border-zinc-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100 dark:focus:border-zinc-500"
											placeholder={m.document_name_placeholder()}
										/>
										{#if isEditingTitle}
											<div class="flex gap-2">
												<button
													type="button"
													class="rounded-lg bg-sky-500 px-3 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-sky-600 dark:bg-sky-500 dark:text-white dark:hover:bg-sky-400"
													onclick={() => void saveTitle()}
												>
													{m.common_save()}
												</button>
												<button
													type="button"
													class="rounded-lg bg-zinc-200 px-3 py-2 text-sm font-medium text-zinc-700 transition hover:bg-zinc-300 dark:bg-zinc-800 dark:text-zinc-200 dark:hover:bg-zinc-700"
													onclick={cancelEditingTitle}
												>
													{m.common_cancel()}
												</button>
											</div>
										{/if}
									</div>
									</div>

									<div class="space-y-3">
									<div class="flex items-center justify-between gap-3">
										<label
											for="document-design-description"
											class="text-xs font-medium text-zinc-500 dark:text-zinc-400"
										>
											{m.editor_document_settings_description_label()}
										</label>
										{#if draftExcerpt.trim() !== documentManualExcerpt.trim()}
											<button
												type="button"
												class="rounded-lg bg-sky-500 px-3 py-1.5 text-xs font-medium text-white shadow-sm transition hover:bg-sky-600 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-sky-500 dark:text-white dark:hover:bg-sky-400"
												onclick={() => void saveExcerpt()}
												disabled={isSavingExcerpt}
											>
												{isSavingExcerpt ? m.common_saving() : m.common_save()}
											</button>
										{/if}
									</div>
									<textarea
										id="document-design-description"
										rows="3"
										bind:value={draftExcerpt}
										class="w-full resize-none rounded-xl border border-zinc-200 bg-white px-4 py-3 text-sm text-zinc-900 outline-none focus:border-zinc-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100 dark:focus:border-zinc-500"
										placeholder={m.editor_document_settings_description_placeholder()}
									></textarea>
									<p class="text-xs leading-6 text-zinc-400 dark:text-zinc-500">
										{m.editor_document_settings_description_hint()}
									</p>
									</div>

									</section>
							{/if}

							{#if canManageMembers || canManagePublic}
								<section
									bind:this={permissionsSection}
									data-section-id="permissions"
									class="scroll-mt-6 space-y-5"
								>
								<div>
									<h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
										{m.editor_document_settings_permissions_title()}
									</h3>
									<p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
										{m.editor_document_settings_permissions_description()}
									</p>
								</div>

								{#if canManagePublic}
									<div class="space-y-3">
										<div class="flex items-center justify-between gap-3">
											<div>
												<p class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
													{m.editor_document_settings_public_label()}
												</p>
												<p class="text-xs text-zinc-500 dark:text-zinc-400">
													{m.editor_document_settings_public_hint()}
												</p>
											</div>
											<div class="flex items-center gap-2">
												<select
													class="w-full max-w-md rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 outline-none focus:border-zinc-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100 dark:focus:border-zinc-500"
													bind:value={draftPublicAccess}
													disabled={isSavingPublicAccess}
													onchange={(event) => void handlePublicAccessSelect(event)}
												>
													<option value="private">{m.editor_document_settings_public_option_private()}</option>
													<option value="authenticated">{m.editor_document_settings_public_option_authenticated()}</option>
													<option value="public">{m.editor_document_settings_public_option_public()}</option>
												</select>
											</div>
										</div>
										<div class="flex items-center gap-2 rounded-xl border border-zinc-200 px-3 py-2 dark:border-zinc-700">
											<input
												type="text"
												readonly
												value={resolvePublicAccessURL()}
												class="min-w-0 flex-1 bg-transparent text-xs text-zinc-700 outline-none dark:text-zinc-300"
											/>
											<button
												type="button"
												class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-zinc-200 text-zinc-600 transition hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
												onclick={() => void copyPublicURL()}
												aria-label={m.editor_document_settings_public_copy_action()}
												title={m.editor_document_settings_public_copy_action()}
											>
												{#if copiedPublicURL}
													<Check class="h-3.5 w-3.5" />
												{:else}
													<Copy class="h-3.5 w-3.5" />
												{/if}
											</button>
										</div>
									</div>
								{/if}

								{#if canManageMembers}
									<DocumentCollaborationSettings
										{documentId}
										enabled={open && activeSection === 'permissions'}
									/>
								{/if}
								</section>
							{/if}

							{#if canEditImageSettings}
								<section
									bind:this={imageSection}
									data-section-id="image"
									class="scroll-mt-6 space-y-5"
								>
								<div>
									<h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
										{m.editor_document_settings_image_title()}
									</h3>
									<p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
										{m.editor_document_settings_image_description()}
									</p>
								</div>

								<div class="space-y-4">
									<div class="space-y-2">
										<label
											for="image-target-select"
											class="flex items-center gap-2 text-xs font-medium text-zinc-500 dark:text-zinc-400"
										>
											<ImageSquare class="h-4 w-4" />
											{m.editor_document_settings_image_upload_target()}
										</label>
										<select
											id="image-target-select"
											value={currentTargetId}
											onchange={(event) => handleSelect((event.currentTarget as HTMLSelectElement).value)}
											disabled={isUpdating}
											class="w-full max-w-md rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 outline-none focus:border-zinc-400 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100 dark:focus:border-zinc-500"
										>
											{#each options as option (option.id)}
												<option value={option.id}>{option.label}</option>
											{/each}
										</select>
									</div>

									<div class="space-y-2 text-xs leading-5 text-zinc-400 dark:text-zinc-500">
										<p>
											{m.editor_document_settings_image_help_private()}<br />
											{m.editor_document_settings_image_help_public()}<br />
											{m.editor_document_settings_image_help_migrate()}
										</p>
									</div>
								</div>
								</section>
							{/if}
						</div>
					</section>
				</div>
			</div>
		</div>
	</div>
{/if}
