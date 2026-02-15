<script>
  export let message = '';
</script>

<div class="error-banner">
  <span class="error-icon">⚠</span>
  <div>
    <strong>Setup Required</strong>
    <p>{message}</p>
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
</style>
