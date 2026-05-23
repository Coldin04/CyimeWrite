<script lang="ts">
	import { browser } from '$app/environment';
	import Logo from '$lib/components/common/Logo.svelte';
	import {
		clearManualLocaleCookie,
		getManualLocaleFromDocument,
		setManualLocaleCookie
	} from '$lib/paraglide/manual-locale-cookie';
	import * as m from '$paraglide/messages';
	import { getLocale, isLocale, locales } from '$paraglide/runtime';
	import GlobeHemisphereWest from '~icons/ph/globe-hemisphere-west';
	import GithubLogo from '~icons/ph/github-logo';
	import previewImageLightUrl from '$lib/assets/home_preview_light.png';
	import previewImageDarkUrl from '$lib/assets/home_preview_dark.png';
	import { onMount } from 'svelte';

	const homepageHeroHeadlinePhrases = [
		m.homepage_hero_word_light(),
		m.homepage_hero_word_flow(),
		m.homepage_hero_word_ai(),
		m.homepage_hero_word_export()
	];

	let homepageHeroHeadlinePhrase = homepageHeroHeadlinePhrases[0];
	let homepageHeroHeadlinePhraseIndex = 0;
	type LocalePreference = 'system' | (typeof locales)[number];
	let localePreference: LocalePreference = 'system';
	let localeMenuOpen = false;
	let localeMenuElement: HTMLDivElement | null = null;

	function getLocaleOptionLabel(localeTag: string): string {
		if (!browser || typeof Intl === 'undefined' || typeof Intl.DisplayNames === 'undefined') {
			return localeTag;
		}
		try {
			const display = new Intl.DisplayNames([getLocale()], { type: 'language' });
			return display.of(localeTag.split('-')[0]) ?? localeTag;
		} catch {
			return localeTag;
		}
	}

	function handleLocaleSelect(next: LocalePreference) {
		if (next === localePreference) {
			localeMenuOpen = false;
			return;
		}

		if (next === 'system') {
			clearManualLocaleCookie();
			localePreference = 'system';
			localeMenuOpen = false;
			if (browser) window.location.reload();
			return;
		}

		setManualLocaleCookie(next);
		localePreference = next;
		localeMenuOpen = false;
		if (browser) window.location.reload();
	}

	onMount(() => {
		let rotationTimeout: ReturnType<typeof setTimeout>;
		const manualLocale = getManualLocaleFromDocument();
		localePreference = manualLocale && isLocale(manualLocale) ? manualLocale : 'system';
		const handlePointerDown = (event: PointerEvent) => {
			if (!localeMenuElement?.contains(event.target as Node)) {
				localeMenuOpen = false;
			}
		};

		const scheduleNextPhraseRotation = () => {
			rotationTimeout = setTimeout(() => {
				homepageHeroHeadlinePhraseIndex =
					(homepageHeroHeadlinePhraseIndex + 1) % homepageHeroHeadlinePhrases.length;
				homepageHeroHeadlinePhrase =
					homepageHeroHeadlinePhrases[homepageHeroHeadlinePhraseIndex];
				scheduleNextPhraseRotation();
			}, 3000);
		};

		scheduleNextPhraseRotation();
		if (browser) {
			window.addEventListener('pointerdown', handlePointerDown);
		}

		return () => {
			clearTimeout(rotationTimeout);
			if (browser) {
				window.removeEventListener('pointerdown', handlePointerDown);
			}
		};
	});
</script>

<svelte:head>
  <title>{m.page_title_homepage()}</title>
  <meta name="description" content={m.homepage_meta_description()} />
  <meta name="keywords" content={m.homepage_meta_keywords()} />
  <meta property="og:title" content={m.page_title_homepage()} />
  <meta property="og:description" content={m.homepage_meta_description()} />
  <meta name="twitter:title" content={m.page_title_homepage()} />
  <meta name="twitter:description" content={m.homepage_meta_description()} />
</svelte:head>

<nav
	class="sticky top-0 z-30 bg-white/80 py-2 pt-[calc(env(safe-area-inset-top)+0.5rem)] backdrop-blur-md dark:bg-slate-900/80"
>
	<div class="mx-auto flex h-16 w-full max-w-6xl items-center justify-between px-6 sm:px-8">
		<Logo href="/" labelClass="text-lg font-bold tracking-tight sm:text-xl" />
		<div class="flex items-center gap-3">
			<div class="relative" bind:this={localeMenuElement}>
				<button
					type="button"
					class="grid h-10 w-10 place-content-center rounded-xl text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-white"
					aria-label={m.user_profile_language_title()}
					title={m.user_profile_language_title()}
					onclick={() => (localeMenuOpen = !localeMenuOpen)}
				>
					<GlobeHemisphereWest class="h-5 w-5" />
				</button>
				{#if localeMenuOpen}
					<div
						class="absolute right-0 top-full z-40 mt-2 min-w-40 rounded-xl bg-white p-1.5 shadow-[0_14px_40px_rgba(15,23,42,0.12)] ring-1 ring-slate-200/80 dark:bg-slate-800 dark:ring-slate-700/80"
					>
						<button
							type="button"
							class={`flex w-full items-center rounded-lg px-3 py-2 text-left text-sm transition-colors ${
								localePreference === 'system'
									? 'bg-sky-50 text-sky-900 dark:bg-sky-950/40 dark:text-sky-100'
									: 'text-slate-600 hover:bg-slate-50 dark:text-slate-300 dark:hover:bg-slate-700/70'
							}`}
							onclick={() => handleLocaleSelect('system')}
						>
							{m.user_profile_language_option_system()}
						</button>
						{#each locales as localeTag (localeTag)}
							<button
								type="button"
								class={`flex w-full items-center rounded-lg px-3 py-2 text-left text-sm transition-colors ${
									localePreference === localeTag
										? 'bg-sky-50 text-sky-900 dark:bg-sky-950/40 dark:text-sky-100'
										: 'text-slate-600 hover:bg-slate-50 dark:text-slate-300 dark:hover:bg-slate-700/70'
								}`}
								onclick={() => handleLocaleSelect(localeTag)}
							>
								{getLocaleOptionLabel(localeTag)}
							</button>
						{/each}
					</div>
				{/if}
			</div>
			<a
				href="https://github.com/Coldin04/Cyime"
				target="_blank"
				rel="noreferrer"
				class="grid h-10 w-10 place-content-center rounded-xl text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-white"
				aria-label="GitHub repository"
				title="GitHub"
			>
				<GithubLogo class="h-5 w-5" />
			</a>
		</div>
	</div>
</nav>

<div class="homepage-hero min-h-screen px-6 pb-8 pt-14 dark:bg-slate-900 sm:px-8 sm:pt-16">
	<div class="mx-auto grid min-h-[calc(100vh-5rem)] w-full max-w-5xl grid-rows-[1fr_auto]">
		<div class="flex flex-col items-center justify-center py-10 pt-8 text-center md:pt-10">
			<h1
				class="max-w-5xl text-5xl font-bold leading-[1.14] tracking-tight text-slate-800 dark:text-slate-100 sm:text-5xl md:leading-[1.08] md:text-6xl"
			>
				{#key `${homepageHeroHeadlinePhraseIndex}-${homepageHeroHeadlinePhrase}`}
					<span
						class="homepage-hero-headline-phrase slide-in bg-gradient-to-r from-teal-400 to-sky-300 bg-clip-text text-transparent"
					>
						{homepageHeroHeadlinePhrase}
					</span>
				{/key}
				<span class="mt-3 block md:mt-4">{m.homepage_hero_suffix()}</span>
			</h1>
			<p class="mt-8 max-w-3xl text-base leading-8 text-slate-500 dark:text-slate-400 md:text-lg">
				{m.homepage_hero_description()}
			</p>
			<div class="mt-8 flex flex-col space-y-4 sm:flex-row sm:space-x-4 sm:space-y-0">
				<a
					href="/workspace"
					class="rounded-xl bg-sky-500 px-6 py-3 font-semibold text-white shadow-lg transition-shadow"
				>
					{m.homepage_start_writing_button()}
				</a>
				<a
					href="#features"
					class="rounded-xl bg-sky-50 px-6 py-3 font-semibold text-slate-800 shadow-lg transition-shadow hover:shadow-xl dark:bg-slate-700 dark:text-gray-300"
				>
					{m.homepage_learn_more_button()}
				</a>
			</div>
		</div>

		<div class="pb-10">
			<div class="mx-auto w-full max-w-4xl px-2 sm:px-0">
				<div class="overflow-hidden rounded-2xl bg-white shadow-[0_18px_60px_rgba(15,23,42,0.12)] ring-1 ring-black/5 dark:bg-slate-800 dark:ring-white/10 dark:shadow-[0_18px_60px_rgba(0,0,0,0.45)]">
					<div class="homepage-preview-frame bg-slate-50 dark:bg-slate-900">
						<picture>
							<source media="(prefers-color-scheme: dark)" srcset={previewImageDarkUrl} />
							<img
								src={previewImageLightUrl}
								alt={m.homepage_editor_features_screenshot_alt()}
								loading="lazy"
								decoding="async"
								class="homepage-preview-image block h-auto w-full"
							/>
						</picture>
						<div class="homepage-preview-mask pointer-events-none absolute inset-0"></div>
					</div>
				</div>
			</div>
		</div>
	</div>
</div>

<!-- Features Section -->
<section id="features" class="px-6 py-16 sm:px-8 md:py-24">
	<div class="mx-auto flex w-full max-w-6xl flex-wrap justify-center gap-x-16 gap-y-14 md:gap-x-20 md:gap-y-16">
		<div class="w-full max-w-[18rem] text-center md:text-left">
			<h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-slate-100 md:text-2xl">
				{m.homepage_online_sync_title()}
			</h2>
			<p class="mt-4 text-sm leading-7 text-slate-500 dark:text-slate-400">
				{m.homepage_online_sync_description()}
			</p>
		</div>
		<div class="w-full max-w-[18rem] text-center md:text-left">
			<h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-slate-100 md:text-2xl">
				{m.homepage_focus_writing_title()}
			</h2>
			<p class="mt-4 text-sm leading-7 text-slate-500 dark:text-slate-400">
				{m.homepage_focus_writing_description()}
			</p>
		</div>
		<div class="w-full max-w-[18rem] text-center md:text-left">
			<h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-slate-100 md:text-2xl">
				{m.homepage_feature_media_title()}
			</h2>
			<p class="mt-4 text-sm leading-7 text-slate-500 dark:text-slate-400">
				{m.homepage_feature_media_desc()}
			</p>
		</div>
		<div class="w-full max-w-[18rem] text-center md:text-left">
			<h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-slate-100 md:text-2xl">
				{m.homepage_feature_export_title()}
			</h2>
			<p class="mt-4 text-sm leading-7 text-slate-500 dark:text-slate-400">
				{m.homepage_feature_export_desc()}
			</p>
		</div>
		<div class="w-full max-w-[18rem] text-center md:text-left">
			<h2 class="text-xl font-bold tracking-tight text-slate-800 dark:text-slate-100 md:text-2xl">
				{m.homepage_feature_ai_title()}
			</h2>
			<p class="mt-4 text-sm leading-7 text-slate-500 dark:text-slate-400">
				{m.homepage_feature_ai_desc()}
			</p>
		</div>
	</div>
</section>

<!-- Footer -->
<footer class="bg-gray-100 dark:bg-slate-700">
	<div class="mx-auto flex max-w-5xl flex-col items-center gap-4 px-6 py-10 text-center sm:px-8">
		<a
			href="https://github.com/Coldin04/Cyime/blob/main/LICENSE"
			target="_blank"
			rel="noreferrer"
			class="text-sm text-slate-500 transition-colors hover:text-slate-900 dark:text-slate-300 dark:hover:text-white"
		>
			{m.homepage_footer_copyright()}
		</a>
		<div class="flex flex-wrap items-center justify-center gap-x-4 gap-y-2 text-sm text-slate-500 dark:text-slate-300">
			<a
				href="https://zread.ai/Coldin04/Cyime"
				target="_blank"
				rel="noreferrer"
				class="transition-colors hover:text-slate-900 dark:hover:text-white"
			>
				{m.homepage_footer_docs()}
			</a>
			<a
				href="https://github.com/Coldin04/Cyime"
				target="_blank"
				rel="noreferrer"
				class="transition-colors hover:text-slate-900 dark:hover:text-white"
			>
				{m.homepage_footer_repository()}
			</a>
			<a
				href="https://github.com/Coldin04/Cyime/issues"
				target="_blank"
				rel="noreferrer"
				class="transition-colors hover:text-slate-900 dark:hover:text-white"
			>
				{m.homepage_footer_issues()}
			</a>
		</div>
	</div>
</footer>

<style>
	@keyframes slideInFromRight {
		0% {
			transform: translateX(1.5rem);
			clip-path: inset(0 0 0 100%);
			opacity: 0;
		}
		45% {
			opacity: 1;
		}
		100% {
			transform: translateX(0);
			clip-path: inset(0 0 0 0);
			opacity: 1;
		}
	}

	.homepage-hero-headline-phrase {
		display: inline-block;
		will-change: transform, clip-path, opacity;
	}

	.homepage-preview-frame {
		position: relative;
	}

	.homepage-preview-mask {
		background: transparent;
		display: none;
	}

	@media (prefers-color-scheme: dark) {
		.homepage-preview-mask {
			background: linear-gradient(180deg, rgb(15 23 42 / 0.04), rgb(15 23 42 / 0.16));
			display: block;
		}
	}

	.slide-in {
		animation: slideInFromRight 0.85s cubic-bezier(0.22, 1, 0.36, 1);
	}
</style>
