<script>
  import { createEventDispatcher } from 'svelte';
  export let message = '';

  const dispatch = createEventDispatcher();
</script>

<div class="error-banner">
  <span class="error-icon">⚠</span>
  <div class="error-content">
    <strong>Setup Required</strong>
    <p>{message}</p>
    <button class="btn-open-settings" on:click={() => dispatch('openSettings')}>
      Open Settings
    </button>
  </div>
</div>

<style>
  .error-banner {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 14px 16px;
    background: rgba(239, 68, 68, 0.08);
    border: 1px solid rgba(239, 68, 68, 0.2);
    border-radius: 10px;
    margin-bottom: 24px;
    color: var(--text-primary);
  }

  .error-content {
    flex: 1;
    min-width: 0;
  }

  .error-icon {
    font-size: 18px;
    flex-shrink: 0;
    margin-top: 1px;
  }

  .error-banner strong {
    display: block;
    margin-bottom: 4px;
    color: #f87171;
    font-size: 14px;
  }

  .error-banner p {
    font-size: 13px;
    color: var(--text-secondary);
    line-height: 1.5;
    margin: 0;
  }

  .btn-open-settings {
    display: inline-flex;
    align-items: center;
    margin-top: 10px;
    padding: 6px 14px;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.3);
    border-radius: 6px;
    color: #f87171;
    font-size: 12px;
    font-weight: 500;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .btn-open-settings:hover {
    background: rgba(239, 68, 68, 0.2);
    border-color: rgba(239, 68, 68, 0.45);
    color: #fca5a5;
  }
</style>
