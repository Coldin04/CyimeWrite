<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import * as m from '$paraglide/messages';
  import { resolveApiUrl } from '$lib/config/api';

  type AuthProvider = {
    name: string;
    displayName?: string;
    icon: string;
    ssoUrl: string;
  };

  let authProviders: AuthProvider[] = [];
  let isLoading = true;
  let error: string | null = null;

  function formatProviderName(name: string): string {
    const value = name.trim();
    if (!value) return name;
    return value.charAt(0).toUpperCase() + value.slice(1);
  }

  function resolveProviderLabel(provider: AuthProvider): string {
    const displayName = provider.displayName?.trim();
    if (displayName) return displayName;
    return formatProviderName(provider.name);
  }

  function providerLoginUrl(provider: AuthProvider): string {
    const returnTo = $page.url.searchParams.get('return_to')?.trim();
    if (!returnTo) return provider.ssoUrl;

    const url = new URL(provider.ssoUrl);
    url.searchParams.set('return_to', returnTo);
    return url.toString();
  }

  onMount(async () => {
    try {
      const response = await fetch(resolveApiUrl('/api/v1/auth/config'), {
        credentials: 'include'
      });
      if (!response.ok) {
        throw new Error(m.sso_login_box_error_fetch_config());
      }
      const data = await response.json();
      authProviders = data.providers || [];
    } catch (e: any) {
      error = e.message;
      console.error('获取认证配置失败:', e);
    } finally {
      isLoading = false;
    }
  });
</script>

<div
  class="min-h-48 w-full rounded-xl bg-white p-8 shadow-[0_0_0_1px_rgba(148,163,184,0.14),0_24px_60px_rgba(15,23,42,0.10),0_0_44px_rgba(34,211,238,0.10)] dark:bg-slate-900 dark:shadow-[0_0_0_1px_rgba(51,65,85,0.65),0_24px_60px_rgba(2,8,23,0.55)]"
>
  <h1 class="mb-4 py-2 text-3xl font-semibold tracking-tight text-slate-700 dark:text-slate-200">
    {m.sso_login_box_title()}
  </h1>
  {#if isLoading}
    <div class="flex w-full h-full items-center justify-center rounded-xl">
      <p class="h-24 py-4 text-slate-500 dark:text-slate-400">{m.sso_login_box_loading_options()}</p>
    </div>
  {:else if error}
    <div class="text-center text-red-500">
      <p>{m.sso_login_box_error_loading_options()}</p>
      <p class="font-mono text-sm">{error}</p>
    </div>
  {:else if authProviders.length > 0}
    <div class="flex flex-col space-y-4 py-2">
      {#each authProviders as provider, i}
        <a
          href={providerLoginUrl(provider)}
          rel="external"
          class="block w-full rounded-lg px-6 py-3 text-center text-base font-medium shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md {i ===
          0
            ? 'bg-cyan-500 text-cyan-50 shadow-[0_12px_30px_rgba(6,182,212,0.22)] hover:bg-cyan-400 dark:bg-cyan-500 dark:text-white dark:shadow-[0_14px_34px_rgba(8,145,178,0.30)] dark:hover:bg-cyan-400'
            : 'bg-white text-slate-600 ring-1 ring-slate-200/80 hover:bg-slate-50 dark:bg-slate-800 dark:text-slate-200 dark:ring-slate-700/80 dark:hover:bg-slate-700'}"
        >
          {m.sso_login_box_login_with_provider({ providerName: resolveProviderLabel(provider) })}
        </a>
      {/each}
    </div>
  {:else}
    <div class="text-center text-slate-500 dark:text-slate-400">
      <p>{m.sso_login_box_no_sso_options()}</p>
      <p class="mt-2 text-sm">{m.sso_login_box_contact_admin_for_config()}</p>
    </div>
  {/if}
</div>
