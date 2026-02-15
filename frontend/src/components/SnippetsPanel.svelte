<script>
  import {
    snippets,
    addSnippet,
    updateSnippet,
    deleteSnippet,
  } from '../lib/stores/snippetsStore.js';

  // ─── Add form state ───
  let newTrigger = '';
  let newExpansion = '';
  let addError = '';
  let addLoading = false;

  // ─── Edit state ───
  let editingId = null;
  let editTrigger = '';
  let editExpansion = '';
  let editError = '';
  let editLoading = false;

  // ─── Hover state ───
  let hoveredId = null;

  // ─── Actions ───
  async function handleAdd() {
    if (!newTrigger.trim() || !newExpansion.trim()) {
      addError = 'Both trigger and expansion are required.';
      return;
    }
    addError = '';
    addLoading = true;
    try {
      await addSnippet(newTrigger.trim(), newExpansion.trim());
      newTrigger = '';
      newExpansion = '';
    } catch (e) {
      addError = e?.message || 'Failed to add snippet.';
    }
    addLoading = false;
  }

  function startEdit(snippet) {
    editingId = snippet.id;
    editTrigger = snippet.trigger;
    editExpansion = snippet.expansion;
    editError = '';
  }

  function cancelEdit() {
    editingId = null;
    editTrigger = '';
    editExpansion = '';
    editError = '';
  }

  async function saveEdit() {
    if (!editTrigger.trim() || !editExpansion.trim()) {
      editError = 'Both fields are required.';
      return;
    }
    editError = '';
    editLoading = true;
    try {
      await updateSnippet(editingId, editTrigger.trim(), editExpansion.trim());
      editingId = null;
    } catch (e) {
      editError = e?.message || 'Failed to update snippet.';
    }
    editLoading = false;
  }

  async function handleDelete(id) {
    await deleteSnippet(id);
    if (editingId === id) cancelEdit();
  }

  function handleAddKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleAdd();
    }
  }

  function handleEditKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      saveEdit();
    }
    if (e.key === 'Escape') {
      cancelEdit();
    }
  }


</script>

<div class="snippets-panel">
  <!-- Header -->
  <div class="snippets-header">
    <h2 class="snippets-title">Snippets</h2>
    <span class="snippets-count">{$snippets.length}</span>
  </div>

  <p class="snippets-desc">
    Snippets auto-replace trigger phrases in your dictation. Say the trigger and it expands to the full text.
  </p>

  <!-- Add new snippet form -->
  <div class="add-form">
    <div class="add-form-header">
      <span>Add new snippet</span>
    </div>
    <div class="add-form-fields">
      <div class="field-group">
        <label class="field-label" for="new-trigger">Trigger</label>
        <input
          id="new-trigger"
          class="field-input"
          type="text"
          placeholder="e.g. my calendly link"
          bind:value={newTrigger}
          on:keydown={handleAddKeydown}
          disabled={addLoading}
        />
      </div>
      <div class="field-arrow">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M5 12h14M12 5l7 7-7 7"/>
        </svg>
      </div>
      <div class="field-group field-group-expand">
        <label class="field-label" for="new-expansion">Expansion</label>
        <input
          id="new-expansion"
          class="field-input"
          type="text"
          placeholder="e.g. calendly.com/you/invite-name"
          bind:value={newExpansion}
          on:keydown={handleAddKeydown}
          disabled={addLoading}
        />
      </div>
      <button
        class="btn-add"
        on:click={handleAdd}
        disabled={addLoading || !newTrigger.trim() || !newExpansion.trim()}
        title="Add snippet"
      >
        {#if addLoading}
          <div class="btn-spinner"></div>
        {:else}
          Add
        {/if}
      </button>
    </div>
    {#if addError}
      <div class="form-error">{addError}</div>
    {/if}
  </div>

  <!-- Snippet list -->
  <div class="snippets-list">
    {#if $snippets.length === 0}
      <div class="empty-state">
        <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" opacity="0.3">
          <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/>
          <polyline points="14 2 14 8 20 8"/>
          <line x1="16" y1="13" x2="8" y2="13"/>
          <line x1="16" y1="17" x2="8" y2="17"/>
        </svg>
        <p>No snippets yet</p>
        <span>Add a trigger phrase above to get started.</span>
      </div>
    {:else}
      {#each $snippets as snippet (snippet.id)}
        <div
          class="snippet-row"
          class:editing={editingId === snippet.id}
          on:mouseenter={() => hoveredId = snippet.id}
          on:mouseleave={() => hoveredId = null}
        >
          {#if editingId === snippet.id}
            <!-- Edit mode -->
            <div class="edit-fields">
              <div class="field-group">
                <input
                  class="field-input"
                  type="text"
                  bind:value={editTrigger}
                  on:keydown={handleEditKeydown}
                  disabled={editLoading}
                  placeholder="Trigger"
                />
              </div>
              <div class="field-arrow">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M5 12h14M12 5l7 7-7 7"/>
                </svg>
              </div>
              <div class="field-group field-group-expand">
                <input
                  class="field-input"
                  type="text"
                  bind:value={editExpansion}
                  on:keydown={handleEditKeydown}
                  disabled={editLoading}
                  placeholder="Expansion"
                />
              </div>
              <div class="edit-actions">
                <button class="btn-save" on:click={saveEdit} disabled={editLoading} title="Save">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                    <path d="M20 6L9 17l-5-5"/>
                  </svg>
                </button>
                <button class="btn-cancel" on:click={cancelEdit} title="Cancel">
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                    <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
                  </svg>
                </button>
              </div>
            </div>
            {#if editError}
              <div class="form-error">{editError}</div>
            {/if}
          {:else}
            <!-- Display mode -->
            <button class="snippet-content" on:click={() => startEdit(snippet)} title="Click to edit">
              <span class="snippet-trigger">{snippet.trigger}</span>
              <svg class="snippet-arrow" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M5 12h14M12 5l7 7-7 7"/>
              </svg>
              <span class="snippet-expansion">{snippet.expansion}</span>
            </button>
            <button
              class="btn-delete"
              class:visible={hoveredId === snippet.id}
              on:click|stopPropagation={() => handleDelete(snippet.id)}
              title="Delete snippet"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" />
              </svg>
            </button>
          {/if}
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .snippets-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    max-width: 720px;
    margin: 0 auto;
    width: 100%;
    padding: 20px 24px 24px;
  }

  /* ─── Header ─── */
  .snippets-header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 4px;
  }

  .snippets-title {
    font-size: 18px;
    font-weight: 600;
    color: var(--text-primary);
    margin: 0;
  }

  .snippets-count {
    font-size: 12px;
    color: var(--text-muted);
    background: var(--bg-tertiary);
    padding: 2px 8px;
    border-radius: 10px;
  }

  .snippets-desc {
    font-size: 13px;
    color: var(--text-muted);
    line-height: 1.5;
    margin: 8px 0 20px;
  }

  /* ─── Add Form ─── */
  .add-form {
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 16px;
    margin-bottom: 20px;
  }

  .add-form-header {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-secondary);
    margin-bottom: 12px;
  }

  .add-form-fields {
    display: flex;
    align-items: flex-end;
    gap: 10px;
  }

  .field-group {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }

  .field-group-expand {
    flex: 1.5;
  }

  .field-label {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .field-input {
    padding: 8px 12px;
    background: var(--bg-input);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-primary);
    font-size: 13px;
    font-family: var(--font-sans);
    outline: none;
    transition: border-color 0.15s ease;
    min-width: 0;
  }

  .field-input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--accent-glow);
  }

  .field-input::placeholder {
    color: var(--text-muted);
  }

  .field-input:disabled {
    opacity: 0.5;
  }

  .field-arrow {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    padding-bottom: 8px;
    flex-shrink: 0;
  }

  .btn-add {
    padding: 8px 18px;
    background: var(--accent);
    border: none;
    border-radius: 8px;
    color: #000;
    font-size: 13px;
    font-weight: 600;
    font-family: var(--font-sans);
    cursor: pointer;
    white-space: nowrap;
    flex-shrink: 0;
    transition: all 0.15s ease;
    min-height: 35px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .btn-add:hover:not(:disabled) {
    background: var(--accent-hover);
    transform: scale(1.02);
  }

  .btn-add:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .btn-spinner {
    width: 14px;
    height: 14px;
    border: 2px solid rgba(0, 0, 0, 0.2);
    border-top-color: #000;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  .form-error {
    margin-top: 8px;
    font-size: 12px;
    color: #f87171;
  }

  /* ─── Snippet List ─── */
  .snippets-list {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .snippets-list::-webkit-scrollbar {
    width: 4px;
  }

  .snippets-list::-webkit-scrollbar-track {
    background: transparent;
  }

  .snippets-list::-webkit-scrollbar-thumb {
    background: var(--border);
    border-radius: 2px;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 48px 24px;
    color: var(--text-muted);
    text-align: center;
  }

  .empty-state p {
    margin: 12px 0 4px;
    font-size: 14px;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .empty-state span {
    font-size: 13px;
  }

  /* ─── Snippet Row ─── */
  .snippet-row {
    display: flex;
    align-items: center;
    border-radius: 10px;
    transition: background 0.15s ease;
    position: relative;
  }

  .snippet-row:hover:not(.editing) {
    background: var(--bg-tertiary);
  }

  .snippet-content {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    font-family: var(--font-sans);
    min-width: 0;
  }

  .snippet-trigger {
    display: inline-block;
    padding: 3px 10px;
    background: rgba(14, 240, 216, 0.1);
    border: 1px solid rgba(14, 240, 216, 0.2);
    border-radius: 6px;
    color: var(--accent);
    font-size: 13px;
    font-weight: 500;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .snippet-arrow {
    color: var(--text-muted);
    flex-shrink: 0;
    opacity: 0.5;
  }

  .snippet-expansion {
    flex: 1;
    font-size: 13px;
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .btn-delete {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    padding: 0;
    background: none;
    border: none;
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    transition: opacity 0.15s ease, background 0.15s ease, color 0.15s ease;
    margin-right: 8px;
    opacity: 0;
    pointer-events: none;
    flex-shrink: 0;
  }

  .btn-delete.visible {
    opacity: 1;
    pointer-events: auto;
  }

  .btn-delete:hover {
    background: rgba(239, 68, 68, 0.1);
    color: #ef4444;
  }

  /* ─── Edit mode ─── */
  .snippet-row.editing {
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 12px;
  }

  .edit-fields {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .edit-actions {
    display: flex;
    gap: 4px;
    flex-shrink: 0;
  }

  .btn-save,
  .btn-cancel {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    padding: 0;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .btn-save {
    background: rgba(14, 240, 216, 0.1);
    color: var(--accent);
  }

  .btn-save:hover:not(:disabled) {
    background: rgba(14, 240, 216, 0.2);
  }

  .btn-save:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .btn-cancel {
    background: rgba(255, 255, 255, 0.05);
    color: var(--text-muted);
  }

  .btn-cancel:hover {
    background: rgba(255, 255, 255, 0.1);
    color: var(--text-secondary);
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
