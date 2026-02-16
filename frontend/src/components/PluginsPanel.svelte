<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import {
    commands, skills,
    activeSection, activeItemId, activeItemDetail, detailLoading,
    refreshPlugins, switchSection,
    selectCommand, addCommand, updateCommand, deleteCommand,
    selectSkill, addSkill, updateSkill, deleteSkill,
    clearPluginSelection,
    masterPrompt, masterPromptLoading, loadMasterPrompt, saveMasterPrompt,
    memoryFiles, activeMemoryName, activeMemoryDetail, memoryDetailLoading,
    refreshMemoryFiles, selectMemoryFile, addMemoryFile, saveMemoryFile, deleteMemoryFile, clearMemorySelection,
  } from '../lib/stores/pluginsStore.js';

  const dispatch = createEventDispatcher();

  let editing = false;
  let editName = '';
  let editDescription = '';
  let editBody = '';
  let editError = '';
  let saving = false;

  let adding = false;
  let addName = '';
  let addDescription = '';
  let addBody = '';
  let addError = '';
  let addSaving = false;

  let searchQuery = '';
  let hoveredItemId = null;

  // Master prompt editor state
  let promptBody = '';
  let promptEditing = false;
  let promptSaving = false;
  let promptSaved = false;

  // Memory editor state
  let memEditing = false;
  let memEditBody = '';
  let memEditError = '';
  let memSaving = false;

  let memAdding = false;
  let memAddName = '';
  let memAddBody = '';
  let memAddError = '';
  let memAddSaving = false;

  let memSearchQuery = '';
  let hoveredMemName = null;

  onMount(() => { refreshPlugins(); });

  $: isPluginSection = $activeSection === 'commands' || $activeSection === 'skills';
  $: items = $activeSection === 'commands' ? $commands : $skills;
  $: filteredItems = searchQuery
    ? items.filter(i => i.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
                        (i.description || '').toLowerCase().includes(searchQuery.toLowerCase()))
    : items;
  $: detail = $activeItemDetail;
  $: sectionLabel = $activeSection === 'commands' ? 'Commands' : 'Skills';

  $: filteredMemFiles = memSearchQuery
    ? $memoryFiles.filter(m => m.name.toLowerCase().includes(memSearchQuery.toLowerCase()))
    : $memoryFiles;
  $: memDetail = $activeMemoryDetail;

  function handleSelectItem(id) {
    adding = false;
    if ($activeSection === 'commands') selectCommand(id);
    else selectSkill(id);
    editing = false;
  }

  function handleStartEdit() {
    if (!detail) return;
    editName = detail.name || '';
    editDescription = detail.description || '';
    editBody = detail.body || '';
    editError = '';
    editing = true;
  }

  function handleCancelEdit() { editing = false; editError = ''; }

  async function handleSaveEdit() {
    if (!editName.trim()) { editError = 'Name is required.'; return; }
    editError = '';
    saving = true;
    try {
      if ($activeSection === 'commands') await updateCommand($activeItemId, editName, editDescription, editBody);
      else await updateSkill($activeItemId, editName, editDescription, editBody);
      editing = false;
    } catch (e) { editError = e?.message || 'Failed to save.'; }
    saving = false;
  }

  async function handleDelete(id) {
    if ($activeSection === 'commands') await deleteCommand(id);
    else await deleteSkill(id);
  }

  function handleStartAdd() {
    clearPluginSelection();
    addName = '';
    addDescription = '';
    addBody = `# New ${$activeSection === 'commands' ? 'Command' : 'Skill'}\n\nDescribe the instructions here...\n`;
    addError = '';
    adding = true;
    editing = false;
  }

  function handleCancelAdd() { adding = false; addError = ''; }

  async function handleSaveAdd() {
    if (!addName.trim()) { addError = 'Name is required.'; return; }
    addError = '';
    addSaving = true;
    try {
      let result;
      if ($activeSection === 'commands') result = await addCommand(addName, addDescription, addBody);
      else result = await addSkill(addName, addDescription, addBody);
      adding = false;
      if (result && result.id) handleSelectItem(result.id);
    } catch (e) { addError = e?.message || 'Failed to add.'; }
    addSaving = false;
  }

  // Master prompt handlers
  function handleSwitchToPrompt() {
    switchSection('master_prompt');
    adding = false;
    editing = false;
    promptEditing = false;
    promptSaved = false;
    loadMasterPrompt().then(() => {
      promptBody = $masterPrompt;
    });
  }

  function handleStartPromptEdit() {
    promptBody = $masterPrompt;
    promptEditing = true;
    promptSaved = false;
  }

  function handleCancelPromptEdit() {
    promptEditing = false;
    promptBody = $masterPrompt;
  }

  async function handleSavePrompt() {
    promptSaving = true;
    try {
      await saveMasterPrompt(promptBody);
      promptEditing = false;
      promptSaved = true;
      setTimeout(() => { promptSaved = false; }, 2000);
    } catch (e) {
      console.error('Failed to save prompt:', e);
    }
    promptSaving = false;
  }

  // Memory handlers
  function handleSwitchToMemory() {
    switchSection('memory');
    adding = false;
    editing = false;
    memEditing = false;
    memAdding = false;
    refreshMemoryFiles();
  }

  function handleSelectMemory(name) {
    memAdding = false;
    memEditing = false;
    selectMemoryFile(name);
  }

  function handleStartMemEdit() {
    if (!memDetail) return;
    memEditBody = memDetail.body || '';
    memEditError = '';
    memEditing = true;
  }

  function handleCancelMemEdit() { memEditing = false; memEditError = ''; }

  async function handleSaveMemEdit() {
    memEditError = '';
    memSaving = true;
    try {
      await saveMemoryFile($activeMemoryName, memEditBody);
      memEditing = false;
    } catch (e) { memEditError = e?.message || 'Failed to save.'; }
    memSaving = false;
  }

  function handleStartMemAdd() {
    clearMemorySelection();
    memAddName = '';
    memAddBody = '';
    memAddError = '';
    memAdding = true;
    memEditing = false;
  }

  function handleCancelMemAdd() { memAdding = false; memAddError = ''; }

  async function handleSaveMemAdd() {
    if (!memAddName.trim()) { memAddError = 'Name is required.'; return; }
    memAddError = '';
    memAddSaving = true;
    try {
      const result = await addMemoryFile(memAddName, memAddBody);
      memAdding = false;
      if (result && result.name) handleSelectMemory(result.name);
    } catch (e) { memAddError = e?.message || 'Failed to add.'; }
    memAddSaving = false;
  }

  async function handleDeleteMemory(name) {
    await deleteMemoryFile(name);
  }

  function formatDate(ts) {
    if (!ts) return '';
    return new Date(ts).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }

  function formatSize(bytes) {
    if (!bytes) return '0 B';
    if (bytes < 1024) return bytes + ' B';
    return (bytes / 1024).toFixed(1) + ' KB';
  }
</script>

<div class="plugins-panel">
  <!-- Content area: sidebar + panels -->
  <div class="plugins-content">
  <!-- Left: Section nav -->
  <div class="section-nav">
    <div class="section-nav-top">
      <button class="back-btn" on:click={() => dispatch('close')}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5M12 19l-7-7 7-7"/></svg>
      </button>
      <div class="section-nav-header">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="7" width="20" height="14" rx="2" ry="2"/><path d="M16 7V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v2"/></svg>
        <span>Plugins</span>
      </div>
    </div>
    <div class="section-nav-label">Installed</div>
    <button class="section-btn" class:active={$activeSection === 'commands'} on:click={() => { switchSection('commands'); adding = false; }}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 18l2-2-2-2"/><path d="M8 6L6 8l2 2"/><path d="M14.5 4l-5 16"/></svg>
      Commands
      {#if $commands.length > 0}<span class="section-count">{$commands.length}</span>{/if}
    </button>
    <button class="section-btn" class:active={$activeSection === 'skills'} on:click={() => { switchSection('skills'); adding = false; }}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/><path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/></svg>
      Skills
      {#if $skills.length > 0}<span class="section-count">{$skills.length}</span>{/if}
    </button>

    <div class="section-nav-label">Configure</div>
    <button class="section-btn" class:active={$activeSection === 'master_prompt'} on:click={handleSwitchToPrompt}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
      Master Prompt
    </button>
    <button class="section-btn" class:active={$activeSection === 'memory'} on:click={handleSwitchToMemory}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>
      Memory
      {#if $memoryFiles.length > 0}<span class="section-count">{$memoryFiles.length}</span>{/if}
    </button>
  </div>

  <!-- Master Prompt: full-width editor (no middle list panel) -->
  {#if $activeSection === 'master_prompt'}
    <div class="detail-panel">
      {#if $masterPromptLoading}
        <div class="detail-loading"><div class="spinner"></div></div>
      {:else}
        <div class="detail-header">
          <div class="detail-header-top">
            <h2 class="detail-title">Master Prompt</h2>
            {#if !promptEditing}
              <button class="btn-edit" on:click={handleStartPromptEdit}>Edit</button>
            {/if}
          </div>
          {#if !promptEditing}
            <p class="detail-description-hint">This is the system prompt sent to the AI at the start of every conversation.</p>
          {/if}
        </div>
        {#if promptEditing}
          <div class="edit-form">
            <div class="edit-field edit-field-body prompt-editor-field">
              <label>Content (Markdown)</label>
              <textarea bind:value={promptBody} disabled={promptSaving}></textarea>
            </div>
            <div class="edit-actions">
              <button class="btn-cancel" on:click={handleCancelPromptEdit}>Cancel</button>
              <button class="btn-save" on:click={handleSavePrompt} disabled={promptSaving}>{promptSaving ? 'Saving...' : 'Save'}</button>
            </div>
          </div>
        {:else}
          <div class="detail-body">
            <pre class="detail-body-content">{$masterPrompt || '(empty)'}</pre>
          </div>
        {/if}
        {#if promptSaved}
          <div class="save-toast">Saved</div>
        {/if}
      {/if}
    </div>

  <!-- Memory section: list + detail -->
  {:else if $activeSection === 'memory'}
    <div class="item-list">
      <div class="item-list-header">
        <span class="item-list-title">Memory</span>
        <button class="btn-add-item" on:click={handleStartMemAdd} title="Add new">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>
        </button>
      </div>
      <div class="item-search">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/></svg>
        <input type="text" placeholder="Search memories..." bind:value={memSearchQuery} />
      </div>
      <div class="item-entries">
        {#if filteredMemFiles.length === 0}
          <div class="empty-items"><p>No memories yet</p><span>Click + to add one.</span></div>
        {:else}
          {#each filteredMemFiles as mem (mem.name)}
            <button class="item-entry" class:active={mem.name === $activeMemoryName && !memAdding}
              on:click={() => handleSelectMemory(mem.name)}
              on:mouseenter={() => hoveredMemName = mem.name}
              on:mouseleave={() => hoveredMemName = null}>
              <span class="item-entry-name">{mem.name}</span>
              <button class="item-delete-btn" class:visible={hoveredMemName === mem.name}
                on:click|stopPropagation={() => handleDeleteMemory(mem.name)} title="Delete">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6"/></svg>
              </button>
            </button>
          {/each}
        {/if}
      </div>
    </div>
    <div class="detail-panel">
      {#if memAdding}
        <div class="detail-header"><h2 class="detail-title">New Memory</h2></div>
        <div class="edit-form">
          <div class="edit-field"><label>Name</label><input type="text" bind:value={memAddName} placeholder="e.g. user-preferences" disabled={memAddSaving} /><span class="field-hint">No spaces. Use hyphens or underscores (e.g. project-context)</span></div>
          <div class="edit-field edit-field-body"><label>Content (Markdown)</label><textarea bind:value={memAddBody} placeholder="Write memory content here..." disabled={memAddSaving}></textarea></div>
          {#if memAddError}<div class="edit-error">{memAddError}</div>{/if}
          <div class="edit-actions">
            <button class="btn-cancel" on:click={handleCancelMemAdd}>Cancel</button>
            <button class="btn-save" on:click={handleSaveMemAdd} disabled={memAddSaving || !memAddName.trim()}>{memAddSaving ? 'Saving...' : 'Create'}</button>
          </div>
        </div>
      {:else if $memoryDetailLoading}
        <div class="detail-loading"><div class="spinner"></div></div>
      {:else if memDetail}
        <div class="detail-header">
          <div class="detail-header-top">
            <h2 class="detail-title">{memDetail.name}</h2>
            {#if !memEditing}<button class="btn-edit" on:click={handleStartMemEdit}>Edit</button>{/if}
          </div>
          {#if !memEditing}
            <div class="detail-meta">
              <span class="meta-item"><span class="meta-label">Updated</span><span class="meta-value">{formatDate(memDetail.updatedAt)}</span></span>
              <span class="meta-item"><span class="meta-label">Size</span><span class="meta-value">{formatSize(memDetail.size)}</span></span>
            </div>
          {/if}
        </div>
        {#if memEditing}
          <div class="edit-form">
            <div class="edit-field edit-field-body"><label>Content (Markdown)</label><textarea bind:value={memEditBody} disabled={memSaving}></textarea></div>
            {#if memEditError}<div class="edit-error">{memEditError}</div>{/if}
            <div class="edit-actions">
              <button class="btn-cancel" on:click={handleCancelMemEdit}>Cancel</button>
              <button class="btn-save" on:click={handleSaveMemEdit} disabled={memSaving}>{memSaving ? 'Saving...' : 'Save'}</button>
            </div>
          </div>
        {:else}
          <div class="detail-body"><pre class="detail-body-content">{memDetail.body || '(empty)'}</pre></div>
        {/if}
      {:else}
        <div class="detail-empty">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round" opacity="0.2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>
          <p>Select a memory to view details</p>
          <span>or click + to create a new one</span>
        </div>
      {/if}
    </div>

  <!-- Commands / Skills: original layout -->
  {:else}
    <!-- Middle: Item list -->
    <div class="item-list">
      <div class="item-list-header">
        <span class="item-list-title">{sectionLabel}</span>
        <button class="btn-add-item" on:click={handleStartAdd} title="Add new">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>
        </button>
      </div>
      <div class="item-search">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/></svg>
        <input type="text" placeholder="Search {sectionLabel.toLowerCase()}..." bind:value={searchQuery} />
      </div>
      <div class="item-entries">
        {#if filteredItems.length === 0}
          <div class="empty-items"><p>No {sectionLabel.toLowerCase()} yet</p><span>Click + to add one.</span></div>
        {:else}
          {#each filteredItems as item (item.id)}
            <button class="item-entry" class:active={item.id === $activeItemId && !adding}
              on:click={() => handleSelectItem(item.id)}
              on:mouseenter={() => hoveredItemId = item.id}
              on:mouseleave={() => hoveredItemId = null}>
              <span class="item-entry-name">{item.name}</span>
              <button class="item-delete-btn" class:visible={hoveredItemId === item.id}
                on:click|stopPropagation={() => handleDelete(item.id)} title="Delete">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6"/></svg>
              </button>
            </button>
          {/each}
        {/if}
      </div>
    </div>

    <!-- Right: Detail / Edit / Add -->
    <div class="detail-panel">
      {#if adding}
        <div class="detail-header"><h2 class="detail-title">New {$activeSection === 'commands' ? 'Command' : 'Skill'}</h2></div>
        <div class="edit-form">
          <div class="edit-field"><label>Name</label><input type="text" bind:value={addName} placeholder="e.g. analysis-bs" disabled={addSaving} /><span class="field-hint">No spaces. Use hyphens or underscores (e.g. analysis_bs)</span></div>
          <div class="edit-field"><label>Description</label><input type="text" bind:value={addDescription} placeholder="Short description..." disabled={addSaving} /></div>
          <div class="edit-field edit-field-body"><label>Content (Markdown)</label><textarea bind:value={addBody} placeholder="# Instructions..." disabled={addSaving}></textarea></div>
          {#if addError}<div class="edit-error">{addError}</div>{/if}
          <div class="edit-actions">
            <button class="btn-cancel" on:click={handleCancelAdd}>Cancel</button>
            <button class="btn-save" on:click={handleSaveAdd} disabled={addSaving || !addName.trim()}>{addSaving ? 'Saving...' : 'Create'}</button>
          </div>
        </div>
      {:else if $detailLoading}
        <div class="detail-loading"><div class="spinner"></div></div>
      {:else if detail}
        <div class="detail-header">
          <div class="detail-header-top">
            <h2 class="detail-title">{detail.name}</h2>
            {#if !editing}<button class="btn-edit" on:click={handleStartEdit}>Edit</button>{/if}
          </div>
          {#if !editing}
            <div class="detail-meta">
              <span class="meta-item"><span class="meta-label">Added</span><span class="meta-value">{formatDate(detail.createdAt)}</span></span>
              {#if detail.updatedAt !== detail.createdAt}
                <span class="meta-item"><span class="meta-label">Updated</span><span class="meta-value">{formatDate(detail.updatedAt)}</span></span>
              {/if}
            </div>
            {#if detail.description}<p class="detail-description">{detail.description}</p>{/if}
          {/if}
        </div>
        {#if editing}
          <div class="edit-form">
            <div class="edit-field"><label>Name</label><input type="text" bind:value={editName} disabled={saving} /><span class="field-hint">No spaces. Use hyphens or underscores (e.g. analysis_bs)</span></div>
            <div class="edit-field"><label>Description</label><input type="text" bind:value={editDescription} disabled={saving} /></div>
            <div class="edit-field edit-field-body"><label>Content (Markdown)</label><textarea bind:value={editBody} disabled={saving}></textarea></div>
            {#if editError}<div class="edit-error">{editError}</div>{/if}
            <div class="edit-actions">
              <button class="btn-cancel" on:click={handleCancelEdit}>Cancel</button>
              <button class="btn-save" on:click={handleSaveEdit} disabled={saving || !editName.trim()}>{saving ? 'Saving...' : 'Save'}</button>
            </div>
          </div>
        {:else}
          <div class="detail-body"><pre class="detail-body-content">{detail.body || '(empty)'}</pre></div>
        {/if}
      {:else}
        <div class="detail-empty">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round" opacity="0.2"><rect x="2" y="7" width="20" height="14" rx="2" ry="2"/><path d="M16 7V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v2"/></svg>
          <p>Select a {$activeSection === 'commands' ? 'command' : 'skill'} to view details</p>
          <span>or click + to create a new one</span>
        </div>
      {/if}
    </div>
  {/if}
  </div><!-- end plugins-content -->
</div>

<style>
  .plugins-panel { flex: 1; display: flex; flex-direction: column; overflow: hidden; background: var(--bg-primary); }
  .plugins-content { flex: 1; display: flex; overflow: hidden; }
  .section-nav { --wails-draggable: drag; width: 180px; flex-shrink: 0; display: flex; flex-direction: column; border-right: 1px solid var(--border); padding: 38px 10px 16px; gap: 2px; }
  .section-nav-top { --wails-draggable: no-drag; display: flex; align-items: center; gap: 4px; padding: 0 4px 8px; }
  .back-btn { display: flex; align-items: center; justify-content: center; width: 28px; height: 28px; padding: 0; background: none; border: none; border-radius: 7px; color: var(--text-muted); cursor: pointer; transition: all 0.15s ease; flex-shrink: 0; }
  .back-btn:hover { background: var(--bg-hover); color: var(--text-secondary); }
  .section-nav-header { display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 600; color: var(--text-primary); }
  .section-nav-label { font-size: 10px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.8px; padding: 8px 10px 4px; }
  .section-btn { --wails-draggable: no-drag; display: flex; align-items: center; gap: 8px; width: 100%; padding: 7px 10px; background: none; border: none; border-radius: 7px; color: var(--text-secondary); font-size: 13px; font-family: var(--font-sans); cursor: pointer; transition: all 0.15s ease; text-align: left; }
  .section-btn:hover { background: var(--bg-hover); color: var(--text-primary); }
  .section-btn.active { background: rgba(255,255,255,0.08); color: var(--text-primary); }
  .section-count { margin-left: auto; font-size: 11px; color: var(--text-muted); background: var(--bg-tertiary); padding: 1px 6px; border-radius: 8px; }
  .item-list { width: 220px; flex-shrink: 0; display: flex; flex-direction: column; border-right: 1px solid var(--border); overflow: hidden; padding-top: 38px; }
  .item-list-header { display: flex; align-items: center; justify-content: space-between; padding: 6px 14px 8px; }
  .item-list-title { font-size: 13px; font-weight: 600; color: var(--text-primary); }
  .btn-add-item { display: flex; align-items: center; justify-content: center; width: 26px; height: 26px; background: none; border: 1px solid var(--border); border-radius: 6px; color: var(--text-secondary); cursor: pointer; transition: all 0.15s ease; }
  .btn-add-item:hover { background: var(--bg-hover); color: var(--text-primary); border-color: var(--text-muted); }
  .item-search { display: flex; align-items: center; gap: 6px; padding: 4px 14px 8px; }
  .item-search svg { flex-shrink: 0; color: var(--text-muted); }
  .item-search input { flex: 1; background: none; border: none; outline: none; color: var(--text-secondary); font-size: 12.5px; font-family: var(--font-sans); padding: 4px 0; }
  .item-search input::placeholder { color: var(--text-muted); }
  .item-entries { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 1px; padding: 0 6px; }
  .item-entries::-webkit-scrollbar { width: 4px; }
  .item-entries::-webkit-scrollbar-track { background: transparent; }
  .item-entries::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }
  .empty-items { display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 32px 16px; text-align: center; }
  .empty-items p { font-size: 13px; font-weight: 500; color: var(--text-secondary); margin: 0 0 4px; }
  .empty-items span { font-size: 12px; color: var(--text-muted); }
  .item-entry { display: flex; align-items: center; width: 100%; padding: 7px 10px; background: none; border: none; border-radius: 6px; color: var(--text-secondary); font-size: 13px; font-family: var(--font-sans); cursor: pointer; transition: all 0.12s ease; text-align: left; position: relative; }
  .item-entry:hover { background: var(--bg-hover); color: var(--text-primary); }
  .item-entry.active { background: rgba(255,255,255,0.08); color: var(--text-primary); }
  .item-entry-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .item-delete-btn { flex-shrink: 0; display: flex; align-items: center; justify-content: center; width: 24px; height: 24px; padding: 0; background: none; border: none; border-radius: 4px; color: var(--text-muted); cursor: pointer; transition: all 0.15s ease; opacity: 0; pointer-events: none; }
  .item-delete-btn.visible { opacity: 1; pointer-events: auto; }
  .item-delete-btn:hover { background: rgba(239,68,68,0.1); color: #ef4444; }
  .detail-panel { flex: 1; display: flex; flex-direction: column; overflow: hidden; min-width: 0; position: relative; padding-top: 38px; }
  .detail-header { padding: 20px 24px 0; flex-shrink: 0; }
  .detail-header-top { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .detail-title { font-size: 18px; font-weight: 600; color: var(--text-primary); margin: 0; }
  .detail-meta { display: flex; gap: 16px; margin-top: 8px; }
  .meta-item { display: flex; gap: 6px; font-size: 12px; }
  .meta-label { color: var(--text-muted); }
  .meta-value { color: var(--text-secondary); }
  .detail-description { font-size: 13px; color: var(--text-secondary); line-height: 1.5; margin: 10px 0 0; padding: 10px 14px; background: var(--bg-tertiary); border-radius: 8px; border: 1px solid var(--border); }
  .detail-description-hint { font-size: 12px; color: var(--text-muted); line-height: 1.5; margin: 6px 0 0; }
  .btn-edit { padding: 6px 16px; background: var(--bg-tertiary); border: 1px solid var(--border); border-radius: 8px; color: var(--text-secondary); font-size: 13px; font-weight: 500; font-family: var(--font-sans); cursor: pointer; transition: all 0.15s ease; flex-shrink: 0; }
  .btn-edit:hover { background: var(--bg-hover); color: var(--text-primary); border-color: var(--text-muted); }
  .detail-body { flex: 1; overflow-y: auto; padding: 16px 24px 24px; }
  .detail-body::-webkit-scrollbar { width: 4px; }
  .detail-body::-webkit-scrollbar-track { background: transparent; }
  .detail-body::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }
  .detail-body-content { font-size: 13px; color: var(--text-secondary); line-height: 1.7; font-family: var(--font-mono, 'SF Mono', 'Fira Code', monospace); white-space: pre-wrap; word-wrap: break-word; margin: 0; padding: 16px; background: var(--bg-tertiary); border: 1px solid var(--border); border-radius: 10px; }
  .edit-form { flex: 1; display: flex; flex-direction: column; padding: 16px 24px 24px; gap: 12px; overflow-y: auto; }
  .edit-field { display: flex; flex-direction: column; gap: 4px; }
  .edit-field label { font-size: 11px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.5px; }
  .edit-field input { padding: 8px 12px; background: var(--bg-input, var(--bg-tertiary)); border: 1px solid var(--border); border-radius: 8px; color: var(--text-primary); font-size: 13px; font-family: var(--font-sans); outline: none; transition: border-color 0.15s ease; }
  .edit-field input:focus { border-color: var(--accent); box-shadow: 0 0 0 2px var(--accent-glow, rgba(14,240,216,0.15)); }
  .edit-field input::placeholder { color: var(--text-muted); }
  .edit-field-body { flex: 1; min-height: 200px; display: flex; flex-direction: column; }
  .edit-field textarea { flex: 1; padding: 12px; background: var(--bg-input, var(--bg-tertiary)); border: 1px solid var(--border); border-radius: 8px; color: var(--text-primary); font-size: 13px; font-family: var(--font-mono, 'SF Mono', 'Fira Code', monospace); line-height: 1.6; outline: none; resize: none; transition: border-color 0.15s ease; min-height: 200px; }
  .edit-field textarea:focus { border-color: var(--accent); box-shadow: 0 0 0 2px var(--accent-glow, rgba(14,240,216,0.15)); }
  .edit-field textarea::placeholder { color: var(--text-muted); }
  .prompt-editor-field { min-height: 400px; }
  .prompt-editor-field textarea { min-height: 400px; }
  .field-hint { font-size: 11px; color: var(--text-muted); margin-top: 2px; }
  .edit-error { font-size: 12px; color: #f87171; }
  .edit-actions { display: flex; justify-content: flex-end; gap: 8px; flex-shrink: 0; }
  .btn-cancel { padding: 7px 16px; background: var(--bg-tertiary); border: 1px solid var(--border); border-radius: 8px; color: var(--text-secondary); font-size: 13px; font-family: var(--font-sans); cursor: pointer; transition: all 0.15s ease; }
  .btn-cancel:hover { background: var(--bg-hover); color: var(--text-primary); }
  .btn-save { padding: 7px 20px; background: var(--accent); border: none; border-radius: 8px; color: #000; font-size: 13px; font-weight: 600; font-family: var(--font-sans); cursor: pointer; transition: all 0.15s ease; }
  .btn-save:hover:not(:disabled) { background: var(--accent-hover); transform: scale(1.02); }
  .btn-save:disabled { opacity: 0.4; cursor: not-allowed; }
  .detail-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; color: var(--text-muted); text-align: center; gap: 4px; }
  .detail-empty p { margin: 12px 0 0; font-size: 14px; font-weight: 500; color: var(--text-secondary); }
  .detail-empty span { font-size: 13px; color: var(--text-muted); }
  .detail-loading { flex: 1; display: flex; align-items: center; justify-content: center; }
  .spinner { width: 20px; height: 20px; border: 2px solid var(--border); border-top-color: var(--accent); border-radius: 50%; animation: spin 0.8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .save-toast { position: absolute; bottom: 24px; right: 24px; padding: 8px 20px; background: var(--accent); color: #000; font-size: 13px; font-weight: 600; border-radius: 8px; animation: fadeInOut 2s ease forwards; pointer-events: none; }
  @keyframes fadeInOut { 0% { opacity: 0; transform: translateY(8px); } 15% { opacity: 1; transform: translateY(0); } 80% { opacity: 1; } 100% { opacity: 0; } }
</style>
