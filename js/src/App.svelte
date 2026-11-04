<script lang="ts">
  import { onMount } from "svelte";
  import TopBar from "./components/TopBar.svelte";
  import ToastContainer, { type ToastMessage } from "./components/ToastContainer.svelte";
  import TerminalViewport from "./components/TerminalViewport.svelte";
  import SettingsDrawer from "./components/SettingsDrawer.svelte";
  import {
    DEFAULT_USER_CONFIG,
    createSession,
    deleteSession,
    fetchSessions,
    fetchUserConfig,
    renameSession,
    saveUserConfig,
    uploadFileToSession,
    type SessionTab,
    type UserConfig,
  } from "./api";
  import type { DownloadEvent } from "./osc7777";

  declare var gotty_confirm_close: boolean | undefined;

  let tabs = $state<SessionTab[]>([]);
  let activeTabId = $state<string>("");
  let userConfig = $state<UserConfig>(structuredClone(DEFAULT_USER_CONFIG));
  let settingsOpen = $state(false);
  let toasts = $state<ToastMessage[]>([]);
  let fileInputEl: HTMLInputElement | undefined = $state();

  function addToast(text: string, type: "success" | "error" = "success") {
    const id = Math.random().toString(36).slice(2);
    toasts.push({ id, text, type });
    setTimeout(() => {
      dismissToast(id);
    }, 6000);
  }

  function dismissToast(id: string) {
    toasts = toasts.filter((t) => t.id !== id);
  }

  async function initSessionWorkspace() {
    try {
      const cfg = await fetchUserConfig();
      userConfig = cfg;
    } catch {
      // Keep default config
    }

    try {
      const existing = await fetchSessions();
      if (existing && existing.length > 0) {
        tabs = existing.map((s) => ({ ...s, connected: true }));
        activeTabId = tabs[0].id;
      } else {
        const created = await createSession("Session 1");
        tabs = [{ ...created, connected: true }];
        activeTabId = created.id;
      }
    } catch {
      // Fallback for standalone local mode without X-Forwarded-User header
      tabs = [{ id: "default", name: "Terminal 1", connected: true }];
      activeTabId = "default";
    }
  }

  async function handleNewTab(mode?: string) {
    try {
      const created = await createSession(`Session ${tabs.length + 1}`, mode);
      tabs.push({ ...created, connected: true });
      activeTabId = created.id;
    } catch {
      const fallbackId = `local-${Date.now()}`;
      tabs.push({
        id: fallbackId,
        name: `Session ${tabs.length + 1}`,
        mode: mode || userConfig.session.default_mode || "sandbox",
        connected: true,
      });
      activeTabId = fallbackId;
    }
  }

  async function handleCloseTab(id: string) {
    if (! id.startsWith("local-") && id !== "default") {
      try {
        await deleteSession(id);
      } catch {
        // Ignore if already terminated
      }
    }
    const idx = tabs.findIndex((t) => t.id === id);
    tabs = tabs.filter((t) => t.id !== id);
    if (tabs.length === 0) {
      await handleNewTab();
    } else if (activeTabId === id) {
      const nextIdx = Math.max(0, idx - 1);
      activeTabId = tabs[nextIdx].id;
    }
  }

  async function handleRenameTab(id: string, newName: string) {
    const tab = tabs.find((t) => t.id === id);
    if (tab) {
      tab.name = newName;
    }
    if (!id.startsWith("local-") && id !== "default") {
      try {
        await renameSession(id, newName);
      } catch {
        // Ignore
      }
    }
  }

  function handleSelectTab(id: string) {
    activeTabId = id;
  }

  function handleStatusChange(id: string, connected: boolean) {
    const tab = tabs.find((t) => t.id === id);
    if (tab) {
      tab.connected = connected;
    }
  }

  function handleDownload(dl: DownloadEvent) {
    addToast(`Downloaded ${dl.filename} (${dl.size} bytes)`, "success");
  }

  async function handleSaveConfig(nextCfg: UserConfig) {
    try {
      const saved = await saveUserConfig(nextCfg);
      userConfig = saved;
      addToast("Preferences saved to ~/.gotty/config.yaml", "success");
    } catch {
      userConfig = structuredClone(nextCfg);
      addToast("Preferences applied locally", "success");
    }
  }

  function triggerUploadPicker() {
    fileInputEl?.click();
  }

  async function handleFileInputChange(e: Event) {
    const input = e.target as HTMLInputElement;
    const files = input.files;
    if (!files || files.length === 0 || !activeTabId) return;
    for (let i = 0; i < files.length; i++) {
      const f = files[i];
      try {
        const res = await uploadFileToSession(activeTabId, f);
        addToast(`File written to ${res.path}`, "success");
      } catch (err: any) {
        addToast(`Upload failed: ${err?.message || "Unknown error"}`, "error");
      }
    }
    input.value = "";
  }

  function matchesShortcut(e: KeyboardEvent, shortcut: string): boolean {
    if (!shortcut) return false;
    const parts = shortcut.toLowerCase().split("+").map((p) => p.trim());
    const key = parts[parts.length - 1];
    const wantAlt = parts.includes("alt");
    const wantCtrl = parts.includes("ctrl") || parts.includes("control");
    const wantShift = parts.includes("shift");
    return (
      e.altKey === wantAlt &&
      e.ctrlKey === wantCtrl &&
      e.shiftKey === wantShift &&
      e.key.toLowerCase() === key
    );
  }

  $effect(() => {
    const active = tabs.find((t) => t.id === activeTabId);
    if (active?.name) {
      document.title = `${active.name} - Terminal`;
    } else {
      document.title = "Terminal";
    }
  });

  onMount(() => {
    initSessionWorkspace();

    const keyHandler = (e: KeyboardEvent) => {
      if (matchesShortcut(e, userConfig.keybindings.new_tab_shortcut)) {
        e.preventDefault();
        handleNewTab();
      } else if (matchesShortcut(e, userConfig.keybindings.close_tab_shortcut)) {
        e.preventDefault();
        if (activeTabId) {
          handleCloseTab(activeTabId);
        }
      }
    };

    const beforeUnloadHandler = (event: BeforeUnloadEvent) => {
      const confirmEnabled =
        typeof gotty_confirm_close !== "undefined" ? gotty_confirm_close : true;
      const anyConnected = tabs.some((t) => t.connected !== false);
      if (confirmEnabled && anyConnected) {
        event.preventDefault();
        event.returnValue = "";
      }
    };

    window.addEventListener("keydown", keyHandler);
    window.addEventListener("beforeunload", beforeUnloadHandler);

    return () => {
      window.removeEventListener("keydown", keyHandler);
      window.removeEventListener("beforeunload", beforeUnloadHandler);
    };
  });
</script>

<div class="flex flex-col h-screen w-screen bg-[#11111b] text-[#cdd6f4] overflow-hidden">
  <TopBar
    {tabs}
    {activeTabId}
    defaultMode={userConfig.session.default_mode}
    onSelect={handleSelectTab}
    onClose={handleCloseTab}
    onRename={handleRenameTab}
    onNewTab={handleNewTab}
    onToggleSettings={() => (settingsOpen = !settingsOpen)}
    onUploadClick={triggerUploadPicker}
  />

  <ToastContainer {toasts} onDismiss={dismissToast} />

  <TerminalViewport
    {tabs}
    {activeTabId}
    {userConfig}
    onOsc7777={handleDownload}
    onToast={addToast}
    onStatusChange={handleStatusChange}
  />

  <SettingsDrawer
    open={settingsOpen}
    config={userConfig}
    onClose={() => (settingsOpen = false)}
    onSave={handleSaveConfig}
  />

  <input
    bind:this={fileInputEl}
    type="file"
    multiple
    class="hidden"
    onchange={handleFileInputChange}
  />
</div>
