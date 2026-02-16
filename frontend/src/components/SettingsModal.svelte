<script>
  import { createEventDispatcher } from 'svelte';
  import { GetSettings, SaveSettings, GetStatus, GetDictationStatus, RequestAccessibility, GetExecApprovals, SaveExecApprovals } from '../../wailsjs/go/main/App';

  export let telegramStatus = 'stopped';

  let settingsLoading = false;
  let settingsSaving = false;
  let settingsError = '';
  let settingsSuccess = '';
  let settingsApiKey = '';
  let settingsTelegramToken = '';
  let settingsSpeechProvider = 'whisper';
  let settingsSpeechModel = 'gpt-4o-mini-transcribe';
  let settingsOpenAIKey = '';
  let settingsGeminiKey = '';
  let settingsDeepgramKey = '';
  let settingsHotkeyEnabled = false;
  let settingsHotkeyModifier = 'left_option';
  let dictationAccessibility = false;

  // Command approvals state
  let allowedCommands = [];
  let blockedCommands = [];
  let newAllowedCmd = '';
  let newBlockedCmd = '';

  let activeTab = 'general'; // 'general' | 'voice' | 'commands'

  const dispatch = createEventDispatcher();

  // Load settings when the modal is first created.
  loadSettings();

  async function loadSettings() {
    settingsLoading = true;
    try {
      const s = await GetSettings();
      settingsApiKey = s.anthropic_key || '';
      settingsTelegramToken = s.telegram_token || '';
      settingsSpeechProvider = s.speech_provider || 'whisper';
      settingsSpeechModel = s.speech_model || 'gpt-4o-mini-transcribe';
      settingsOpenAIKey = s.openai_key || '';
      settingsGeminiKey = s.gemini_key || '';
      settingsDeepgramKey = s.deepgram_key || '';
      settingsHotkeyEnabled = s.hotkey_enabled || false;
      settingsHotkeyModifier = s.hotkey_modifier || 'right_option';

      // Load exec approvals (allowed/blocked commands).
      try {
        const approvals = await GetExecApprovals();
        allowedCommands = approvals.allowed || [];
        blockedCommands = approvals.blocked || [];
      } catch (_) {}

      // Check dictation accessibility permission status.
      try {
        const dictStatus = await GetDictationStatus();
        dictationAccessibility = dictStatus.accessibility || false;
      } catch (_) {}
    } catch (e) {
      settingsError = `Failed to load settings: ${e}`;
    } finally {
      settingsLoading = false;
    }
  }

  function close() {
    dispatch('close');
  }

  function addAllowedCmd() {
    const cmd = newAllowedCmd.trim();
    if (cmd && !allowedCommands.includes(cmd)) {
      allowedCommands = [...allowedCommands, cmd];
      newAllowedCmd = '';
    }
  }

  function removeAllowedCmd(index) {
    allowedCommands = allowedCommands.filter((_, i) => i !== index);
  }

  function addBlockedCmd() {
    const cmd = newBlockedCmd.trim();
    if (cmd && !blockedCommands.includes(cmd)) {
      blockedCommands = [...blockedCommands, cmd];
      newBlockedCmd = '';
    }
  }

  function removeBlockedCmd(index) {
    blockedCommands = blockedCommands.filter((_, i) => i !== index);
  }

  function handleAllowedKeydown(e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      addAllowedCmd();
    }
  }

  function handleBlockedKeydown(e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      addBlockedCmd();
    }
  }

  async function save() {
    settingsError = '';
    settingsSuccess = '';
    settingsSaving = true;
    try {
      await SaveSettings({
        anthropic_key: settingsApiKey.trim(),
        telegram_token: settingsTelegramToken.trim(),
        speech_provider: settingsSpeechProvider,
        speech_model: settingsSpeechModel,
        openai_key: settingsOpenAIKey.trim(),
        gemini_key: settingsGeminiKey.trim(),
        deepgram_key: settingsDeepgramKey.trim(),
        hotkey_enabled: settingsHotkeyEnabled,
        hotkey_modifier: settingsHotkeyModifier,
      });

      // Save exec approvals alongside main settings.
      await SaveExecApprovals({
        allowed: allowedCommands,
        blocked: blockedCommands,
      });

      settingsSuccess = 'Settings saved successfully!';

      // Re-check app status in case the key was missing before.
      const status = await GetStatus();
      dispatch('statusUpdate', status);

      // Notify other components (e.g. FlowPanel banner) that settings changed.
      window.dispatchEvent(new CustomEvent('talon:settings-saved'));

      setTimeout(() => {
        settingsSuccess = '';
      }, 2500);
    } catch (e) {
      settingsError = `Failed to save: ${e}`;
    } finally {
      settingsSaving = false;
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') {
      close();
    }
  }
</script>

<div class="modal-overlay" on:click|self={close} on:keydown={handleKeydown} role="presentation">
  <div class="modal" role="dialog" aria-modal="true" aria-label="Settings">
    <div class="modal-header">
      <h2>Settings</h2>
      <button class="btn-close" on:click={close} title="Close">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
          <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
        </svg>
      </button>
    </div>

    {#if settingsLoading}
      <div class="modal-body">
        <p class="settings-loading">Loading settings...</p>
      </div>
    {:else}
      <div class="tab-bar">
        <button
          class="tab-btn"
          class:tab-active={activeTab === 'general'}
          on:click={() => activeTab = 'general'}
          type="button"
        >
          General
        </button>
        <button
          class="tab-btn"
          class:tab-active={activeTab === 'voice'}
          on:click={() => activeTab = 'voice'}
          type="button"
        >
          Voice / STT
        </button>
        <button
          class="tab-btn"
          class:tab-active={activeTab === 'commands'}
          on:click={() => activeTab = 'commands'}
          type="button"
        >
          Commands
        </button>
      </div>

      <div class="modal-body">
        {#if settingsError}
          <div class="settings-alert settings-alert-error">{settingsError}</div>
        {/if}
        {#if settingsSuccess}
          <div class="settings-alert settings-alert-success">{settingsSuccess}</div>
        {/if}

        <!-- ─── General Tab ─── -->
        {#if activeTab === 'general'}
          <label class="field-label" for="settings-api-key">Anthropic API Key</label>
          <input
            id="settings-api-key"
            type="password"
            class="field-input"
            bind:value={settingsApiKey}
            placeholder="sk-ant-api03-..."
            autocomplete="off"
            spellcheck="false"
          />
          <p class="field-hint">Your Anthropic API key. Stored in ~/.talon/config.json</p>

          <label class="field-label" for="settings-telegram-token">Telegram Bot Token</label>
          <input
            id="settings-telegram-token"
            type="password"
            class="field-input"
            bind:value={settingsTelegramToken}
            placeholder="123456:ABC-DEF..."
            autocomplete="off"
            spellcheck="false"
          />
          <p class="field-hint">Used for the Telegram bot (@talonclaw_bot). Saved in ~/.talon/config.json</p>

          <label class="field-label" for="settings-openai-key-general">OpenAI API Key</label>
          <input
            id="settings-openai-key-general"
            type="password"
            class="field-input"
            bind:value={settingsOpenAIKey}
            placeholder="sk-..."
            autocomplete="off"
            spellcheck="false"
          />
          <p class="field-hint">Used for GPT chat models and speech-to-text. Get one at platform.openai.com</p>

          <label class="field-label" for="settings-gemini-key">Google Gemini API Key</label>
          <input
            id="settings-gemini-key"
            type="password"
            class="field-input"
            bind:value={settingsGeminiKey}
            placeholder="AIza..."
            autocomplete="off"
            spellcheck="false"
          />
          <p class="field-hint">Used for Gemini chat models. Get one at aistudio.google.com</p>

          <div class="tg-status-section">
            <div class="tg-status-row">
              <span class="tg-status-label">Telegram Bot</span>
              <span class="tg-status-badge" class:tg-badge-running={telegramStatus === 'running'}>
                <span class="tg-badge-dot" class:tg-badge-dot-active={telegramStatus === 'running'}></span>
                {telegramStatus === 'running' ? 'Running' : 'Stopped'}
              </span>
            </div>
            <p class="field-hint">The bot auto-starts when the app opens with a valid token. You can toggle it from the header.</p>
          </div>
        {/if}

        <!-- ─── Voice / Speech-to-Text Tab ─── -->
        {#if activeTab === 'voice'}
          <label class="field-label" for="settings-speech-provider">Provider</label>
          <div class="provider-toggle">
            <button
              class="provider-btn"
              class:provider-active={settingsSpeechProvider === 'whisper'}
              on:click={() => settingsSpeechProvider = 'whisper'}
              type="button"
            >
              OpenAI Whisper
            </button>
            <button
              class="provider-btn"
              class:provider-active={settingsSpeechProvider === 'deepgram'}
              on:click={() => settingsSpeechProvider = 'deepgram'}
              type="button"
            >
              Deepgram
            </button>
          </div>

          {#if settingsSpeechProvider === 'whisper'}
            <label class="field-label" for="settings-speech-model">Model</label>
            <select
              id="settings-speech-model"
              class="field-input field-select"
              bind:value={settingsSpeechModel}
            >
              <option value="gpt-4o-mini-transcribe">GPT-4o Mini Transcribe (fast, recommended)</option>
              <option value="gpt-4o-transcribe">GPT-4o Transcribe (highest quality)</option>
              <option value="whisper-1">Whisper-1 (legacy)</option>
            </select>
            <p class="field-hint">gpt-4o-mini-transcribe is fast and accurate. gpt-4o-transcribe is highest quality but slower.</p>

            <p class="field-hint" style="margin-top: 12px;">OpenAI API Key is configured in the <button class="link-btn" type="button" on:click={() => activeTab = 'general'}>General</button> tab.</p>
          {:else}
            <label class="field-label" for="settings-deepgram-key">Deepgram API Key</label>
            <input
              id="settings-deepgram-key"
              type="password"
              class="field-input"
              bind:value={settingsDeepgramKey}
              placeholder="dg-..."
              autocomplete="off"
              spellcheck="false"
            />
            <p class="field-hint">Used for Deepgram Nova speech-to-text. Get one at deepgram.com</p>
          {/if}

          <!-- ─── Push-to-Talk Dictation ─── -->
          <div class="dictation-section">
            <h3 class="section-title">Push-to-Talk Dictation</h3>
            <p class="section-desc">Hold a modifier key in any app to record. Release to transcribe and paste text into the focused input field. A mic icon appears in the menu bar.</p>

            <div class="dictation-toggle-row">
              <span class="dictation-toggle-label">Enable Push-to-Talk</span>
              <button
                class="toggle-switch"
                class:toggle-active={settingsHotkeyEnabled}
                on:click={() => settingsHotkeyEnabled = !settingsHotkeyEnabled}
                type="button"
                role="switch"
                aria-checked={settingsHotkeyEnabled}
              >
                <span class="toggle-knob"></span>
              </button>
            </div>

            {#if settingsHotkeyEnabled}
              <label class="field-label" for="settings-hotkey-modifier">Hotkey (hold to record)</label>
              <select
                id="settings-hotkey-modifier"
                class="field-input field-select"
                bind:value={settingsHotkeyModifier}
              >
                <option value="left_option">Left Option (⌥)</option>
                <option value="right_option">Right Option (⌥)</option>
                <option value="left_cmd">Left Command (⌘)</option>
                <option value="right_cmd">Right Command (⌘)</option>
                <option value="left_ctrl">Left Control (⌃)</option>
                <option value="right_ctrl">Right Control (⌃)</option>
              </select>
              <p class="field-hint">Hold this key to record, release to transcribe & paste. Short taps (&lt;300ms) are ignored.</p>

              <div class="accessibility-row">
                <div class="accessibility-info">
                  <span class="accessibility-label">Accessibility Permission</span>
                  <span class="accessibility-badge" class:accessibility-granted={dictationAccessibility}>
                    <span class="accessibility-dot" class:accessibility-dot-active={dictationAccessibility}></span>
                    {dictationAccessibility ? 'Granted' : 'Not Granted'}
                  </span>
                </div>
                {#if !dictationAccessibility}
                  <button
                    class="btn-action-sm"
                    on:click={async () => {
                      const granted = await RequestAccessibility();
                      dictationAccessibility = granted;
                    }}
                    type="button"
                  >
                    Request Permission
                  </button>
                {/if}
              </div>
              <p class="field-hint">
                {dictationAccessibility
                  ? 'Talon can paste transcribed text into other apps.'
                  : 'Required for pasting text into other apps. Open System Settings → Privacy & Security → Accessibility and add Talon.'}
              </p>
            {/if}
          </div>
        {/if}

        <!-- ─── Commands Tab ─── -->
        {#if activeTab === 'commands'}
          <div class="commands-section">
            <h3 class="section-title">Allowed Commands</h3>
            <p class="section-desc">
              Command prefixes the agent is allowed to run via <code>run_command</code>.
              If a command doesn't start with one of these prefixes, it will be blocked.
            </p>

            <div class="cmd-list">
              {#each allowedCommands as cmd, i}
                <div class="cmd-chip">
                  <span class="cmd-chip-text">{cmd}</span>
                  <button
                    class="cmd-chip-remove"
                    on:click={() => removeAllowedCmd(i)}
                    title="Remove"
                    type="button"
                  >
                    <svg width="12" height="12" viewBox="0 0 16 16" fill="none">
                      <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
                    </svg>
                  </button>
                </div>
              {/each}
            </div>

            <div class="cmd-add-row">
              <input
                type="text"
                class="field-input cmd-add-input"
                bind:value={newAllowedCmd}
                on:keydown={handleAllowedKeydown}
                placeholder="e.g. npm, docker, python..."
                spellcheck="false"
              />
              <button class="btn-action-sm" on:click={addAllowedCmd} type="button">Add</button>
            </div>
          </div>

          <div class="commands-section">
            <h3 class="section-title">Blocked Patterns</h3>
            <p class="section-desc">
              Command patterns that are always blocked, even if they match an allowed prefix.
              Use these as a safety net for dangerous commands.
            </p>

            <div class="cmd-list">
              {#each blockedCommands as cmd, i}
                <div class="cmd-chip cmd-chip-blocked">
                  <span class="cmd-chip-text">{cmd}</span>
                  <button
                    class="cmd-chip-remove"
                    on:click={() => removeBlockedCmd(i)}
                    title="Remove"
                    type="button"
                  >
                    <svg width="12" height="12" viewBox="0 0 16 16" fill="none">
                      <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
                    </svg>
                  </button>
                </div>
              {/each}
            </div>

            <div class="cmd-add-row">
              <input
                type="text"
                class="field-input cmd-add-input"
                bind:value={newBlockedCmd}
                on:keydown={handleBlockedKeydown}
                placeholder="e.g. rm -rf /, mkfs..."
                spellcheck="false"
              />
              <button class="btn-action-sm" on:click={addBlockedCmd} type="button">Add</button>
            </div>
          </div>

          <p class="field-hint" style="margin-top: 8px;">
            Changes are saved to <code>~/.talon/exec-approvals.json</code> and take effect immediately.
          </p>
        {/if}
      </div>

      <div class="modal-footer">
        <button class="btn-cancel" on:click={close}>Cancel</button>
        <button class="btn-save" on:click={save} disabled={settingsSaving}>
          {settingsSaving ? 'Saving...' : 'Save'}
        </button>
      </div>
    {/if}
  </div>
</div>

<style>
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
    animation: fadeIn 0.15s ease;
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to   { opacity: 1; }
  }

  .modal {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 14px;
    width: 480px;
    max-width: calc(100vw - 32px);
    max-height: calc(100vh - 64px);
    overflow-y: auto;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
    animation: modalSlideUp 0.2s ease;
  }

  @keyframes modalSlideUp {
    from { opacity: 0; transform: translateY(12px) scale(0.98); }
    to   { opacity: 1; transform: translateY(0) scale(1); }
  }

  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 18px 20px 0;
  }

  .modal-header h2 {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-primary);
    margin: 0;
  }

  .btn-close {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    background: transparent;
    border: none;
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .btn-close:hover {
    background: var(--bg-tertiary);
    color: var(--text-primary);
  }

  /* ─── Tab Bar ─── */
  .tab-bar {
    display: flex;
    gap: 0;
    padding: 0 20px;
    margin-top: 12px;
    border-bottom: 1px solid var(--border);
  }

  .tab-btn {
    position: relative;
    padding: 10px 16px;
    background: transparent;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--text-muted);
    font-size: 13px;
    font-weight: 500;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
    margin-bottom: -1px;
  }

  .tab-btn:hover {
    color: var(--text-secondary);
  }

  .tab-btn.tab-active {
    color: var(--text-primary);
    border-bottom-color: var(--accent);
  }

  .modal-body {
    padding: 20px;
  }

  .settings-loading {
    text-align: center;
    color: var(--text-secondary);
    font-size: 14px;
    padding: 24px 0;
  }

  .settings-alert {
    padding: 10px 14px;
    border-radius: 8px;
    font-size: 13px;
    margin-bottom: 16px;
  }

  .settings-alert-error {
    background: rgba(239, 68, 68, 0.1);
    border: 1px solid rgba(239, 68, 68, 0.25);
    color: #f87171;
  }

  .settings-alert-success {
    background: rgba(34, 197, 94, 0.1);
    border: 1px solid rgba(34, 197, 94, 0.25);
    color: #4ade80;
  }

  .field-label {
    display: block;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-primary);
    margin-bottom: 6px;
    margin-top: 16px;
  }

  .field-label:first-of-type {
    margin-top: 0;
  }

  .field-input {
    width: 100%;
    padding: 10px 12px;
    background: var(--bg-primary);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 13px;
    outline: none;
    transition: border-color 0.15s ease, box-shadow 0.15s ease;
  }

  .field-input::placeholder {
    color: var(--text-muted);
  }

  .field-input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-glow);
  }

  .field-hint {
    font-size: 11px;
    color: var(--text-muted);
    margin-top: 4px;
    line-height: 1.4;
  }

  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 0 20px 20px;
  }

  .btn-cancel {
    padding: 8px 16px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-secondary);
    font-size: 13px;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .btn-cancel:hover {
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border-color: var(--text-muted);
  }

  .btn-save {
    padding: 8px 20px;
    background: var(--accent);
    border: none;
    border-radius: 8px;
    color: #000;
    font-size: 13px;
    font-weight: 500;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .btn-save:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .btn-save:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .section-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
    margin: 0 0 12px;
  }

  .provider-toggle {
    display: flex;
    gap: 4px;
    background: var(--bg-primary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 3px;
    margin-bottom: 12px;
  }

  .provider-btn {
    flex: 1;
    padding: 7px 12px;
    background: transparent;
    border: none;
    border-radius: 6px;
    color: var(--text-muted);
    font-size: 13px;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .provider-btn:hover {
    color: var(--text-secondary);
  }

  .provider-btn.provider-active {
    background: var(--bg-tertiary);
    color: var(--text-primary);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
  }

  .field-select {
    appearance: none;
    -webkit-appearance: none;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%23888' stroke-width='2.5' stroke-linecap='round'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right 12px center;
    padding-right: 36px;
    cursor: pointer;
  }

  .field-select option {
    background: var(--bg-secondary);
    color: var(--text-primary);
  }

  /* ─── Dictation Anywhere ─── */
  .dictation-section {
    margin-top: 24px;
    padding-top: 20px;
    border-top: 1px solid var(--border);
  }

  .section-desc {
    font-size: 12px;
    color: var(--text-muted);
    margin: 0 0 14px;
    line-height: 1.5;
  }

  .dictation-toggle-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 14px;
  }

  .dictation-toggle-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-primary);
  }

  .toggle-switch {
    position: relative;
    width: 44px;
    height: 24px;
    background: var(--bg-primary);
    border: 1px solid var(--border);
    border-radius: 12px;
    cursor: pointer;
    transition: all 0.2s ease;
    padding: 0;
    flex-shrink: 0;
  }

  .toggle-switch.toggle-active {
    background: var(--accent);
    border-color: var(--accent);
  }

  .toggle-knob {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: var(--text-muted);
    transition: all 0.2s ease;
  }

  .toggle-switch.toggle-active .toggle-knob {
    left: 22px;
    background: white;
  }

  .accessibility-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 14px;
    margin-bottom: 4px;
  }

  .accessibility-info {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .accessibility-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-primary);
  }

  .accessibility-badge {
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 3px 8px;
    border-radius: 10px;
    font-size: 11px;
    font-weight: 500;
    background: rgba(239, 68, 68, 0.08);
    border: 1px solid rgba(239, 68, 68, 0.2);
    color: #f87171;
  }

  .accessibility-badge.accessibility-granted {
    background: rgba(34, 197, 94, 0.08);
    border-color: rgba(34, 197, 94, 0.25);
    color: #4ade80;
  }

  .accessibility-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: #f87171;
  }

  .accessibility-dot-active {
    background: #4ade80;
  }

  .link-btn {
    background: none;
    border: none;
    padding: 0;
    color: var(--accent);
    font-size: inherit;
    font-family: inherit;
    cursor: pointer;
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .link-btn:hover {
    color: var(--accent-hover);
  }

  .btn-action-sm {
    padding: 5px 12px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-secondary);
    font-size: 12px;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
    white-space: nowrap;
  }

  .btn-action-sm:hover {
    background: var(--bg-hover);
    border-color: var(--border-light);
    color: var(--text-primary);
  }

  /* ─── Telegram Status in Settings ─── */
  .tg-status-section {
    margin-top: 20px;
    padding-top: 16px;
    border-top: 1px solid var(--border);
  }

  .tg-status-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 6px;
  }

  .tg-status-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-primary);
  }

  .tg-status-badge {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 12px;
    font-size: 12px;
    font-weight: 500;
    background: rgba(255, 255, 255, 0.04);
    border: 1px solid var(--border);
    color: var(--text-secondary);
  }

  .tg-status-badge.tg-badge-running {
    background: rgba(34, 197, 94, 0.08);
    border-color: rgba(34, 197, 94, 0.25);
    color: #4ade80;
  }

  .tg-badge-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-muted);
  }

  .tg-badge-dot-active {
    background: #4ade80;
  }

  /* ─── Commands Tab ─── */
  .commands-section {
    margin-bottom: 20px;
  }

  .commands-section:last-of-type {
    margin-bottom: 0;
  }

  .commands-section .section-title {
    margin-top: 0;
  }

  .commands-section .section-desc {
    margin-bottom: 10px;
  }

  .commands-section .section-desc code {
    font-family: var(--font-mono);
    font-size: 11px;
    background: var(--bg-primary);
    padding: 1px 5px;
    border-radius: 4px;
    border: 1px solid var(--border);
  }

  .cmd-list {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 10px;
    min-height: 28px;
  }

  .cmd-chip {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 8px 4px 10px;
    background: rgba(34, 197, 94, 0.08);
    border: 1px solid rgba(34, 197, 94, 0.2);
    border-radius: 6px;
    font-size: 12px;
    font-family: var(--font-mono);
    color: #4ade80;
    transition: all 0.15s ease;
  }

  .cmd-chip:hover {
    background: rgba(34, 197, 94, 0.14);
  }

  .cmd-chip-blocked {
    background: rgba(239, 68, 68, 0.08);
    border-color: rgba(239, 68, 68, 0.2);
    color: #f87171;
  }

  .cmd-chip-blocked:hover {
    background: rgba(239, 68, 68, 0.14);
  }

  .cmd-chip-text {
    line-height: 1;
  }

  .cmd-chip-remove {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    padding: 0;
    background: transparent;
    border: none;
    border-radius: 3px;
    color: inherit;
    opacity: 0.5;
    cursor: pointer;
    transition: opacity 0.15s ease;
  }

  .cmd-chip-remove:hover {
    opacity: 1;
  }

  .cmd-add-row {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .cmd-add-input {
    flex: 1;
    font-family: var(--font-mono) !important;
  }
</style>
