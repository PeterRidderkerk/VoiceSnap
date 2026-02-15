<script lang="ts">
  import { Call } from '@wailsio/runtime'
  import { t } from '../../lib/i18n'

  interface HistoryEntry {
    text: string
    timestamp: number
  }

  let entries = $state<HistoryEntry[]>([])
  let retentionDays = $state(30)
  let copiedTs = $state<number | null>(null)
  let showConfirm = $state(false)

  const retentionOptions = [
    { value: 7, label: () => t('history.days7') },
    { value: 30, label: () => t('history.days30') },
    { value: 90, label: () => t('history.days90') },
    { value: 0, label: () => t('history.forever') },
  ]

  async function loadHistory() {
    try {
      const list: any = await Call.ByName('voicesnap/services.HistoryService.GetAll')
      entries = list || []
    } catch {
      entries = []
    }
    try {
      const days: any = await Call.ByName('voicesnap/services.HistoryService.GetRetentionDays')
      retentionDays = days || 30
    } catch {}
  }

  async function setRetention(days: number) {
    retentionDays = days
    try {
      await Call.ByName('voicesnap/services.HistoryService.SetRetentionDays', days)
      await loadHistory()
    } catch {}
  }

  async function copyText(entry: HistoryEntry) {
    try {
      await navigator.clipboard.writeText(entry.text)
      copiedTs = entry.timestamp
      setTimeout(() => { if (copiedTs === entry.timestamp) copiedTs = null }, 1500)
    } catch {}
  }

  async function deleteEntry(timestamp: number) {
    try {
      await Call.ByName('voicesnap/services.HistoryService.Delete', timestamp)
      entries = entries.filter(e => e.timestamp !== timestamp)
    } catch {}
  }

  async function clearAll() {
    showConfirm = false
    try {
      await Call.ByName('voicesnap/services.HistoryService.ClearAll')
      entries = []
    } catch {}
  }

  function formatTime(ts: number): string {
    const date = new Date(ts)
    const now = new Date()
    const hours = date.getHours().toString().padStart(2, '0')
    const mins = date.getMinutes().toString().padStart(2, '0')
    const time = `${hours}:${mins}`

    const isToday = date.toDateString() === now.toDateString()
    if (isToday) return time

    const yesterday = new Date(now)
    yesterday.setDate(yesterday.getDate() - 1)
    if (date.toDateString() === yesterday.toDateString()) {
      return `${t('history.yesterday')} ${time}`
    }

    const month = (date.getMonth() + 1).toString()
    const day = date.getDate().toString()
    return `${month}/${day} ${time}`
  }

  // Load on mount
  loadHistory()
</script>

<div class="page">
  <!-- Retention setting -->
  <div class="section retention-section">
    <div class="setting-row">
      <span class="setting-label">{t('history.retention')}</span>
      <div class="retention-pills">
        {#each retentionOptions as opt}
          <button
            class="pill"
            class:active={retentionDays === opt.value}
            onclick={() => setRetention(opt.value)}
          >
            {opt.label()}
          </button>
        {/each}
      </div>
    </div>
  </div>

  <!-- History list -->
  <div class="section list-section">
    {#if entries.length === 0}
      <div class="empty">
        <p class="empty-title">{t('history.empty')}</p>
        <p class="empty-desc">{t('history.emptyDesc')}</p>
      </div>
    {:else}
      {#each entries as entry, i}
        {#if i > 0}
          <div class="divider"></div>
        {/if}
        <div class="history-row">
          <div class="history-content">
            <span class="history-time">{formatTime(entry.timestamp)}</span>
            <span class="history-text">{entry.text}</span>
          </div>
          <div class="history-actions">
            <button class="action-btn copy-btn" onclick={() => copyText(entry)}>
              {#if copiedTs === entry.timestamp}
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                  <path d="M2 7L5.5 10.5L12 4" stroke="var(--color-green)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              {:else}
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                  <rect x="5" y="5" width="7.5" height="7.5" rx="1.5" stroke="currentColor" stroke-width="1.2"/>
                  <path d="M9 5V3C9 2.17 8.33 1.5 7.5 1.5H3C2.17 1.5 1.5 2.17 1.5 3V7.5C1.5 8.33 2.17 9 3 9H5" stroke="currentColor" stroke-width="1.2"/>
                </svg>
              {/if}
            </button>
            <button class="action-btn delete-btn" onclick={() => deleteEntry(entry.timestamp)}>
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                <path d="M3 3.5L11 11.5M11 3.5L3 11.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
              </svg>
            </button>
          </div>
        </div>
      {/each}
    {/if}
  </div>

  <!-- Clear all -->
  {#if entries.length > 0}
    <div class="footer">
      {#if showConfirm}
        <span class="confirm-text">{t('history.clearConfirm')}</span>
        <button class="clear-btn confirm" onclick={clearAll}>{t('history.clearAll')}</button>
        <button class="cancel-btn" onclick={() => showConfirm = false}>×</button>
      {:else}
        <button class="clear-btn" onclick={() => showConfirm = true}>{t('history.clearAll')}</button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .section {
    background: var(--color-bg-grouped-secondary);
    border-radius: var(--radius-md);
    padding: var(--spacing-lg);
    margin-bottom: var(--spacing-md);
  }

  /* Retention */
  .retention-section .setting-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .setting-label {
    font-size: var(--font-size-base);
    font-weight: 500;
  }

  .retention-pills {
    display: flex;
    gap: 6px;
  }

  .pill {
    padding: 4px 12px;
    border-radius: 14px;
    border: 1px solid var(--color-separator);
    background: transparent;
    font-size: var(--font-size-xs);
    color: var(--color-secondary-label);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .pill:hover {
    border-color: var(--color-blue);
    color: var(--color-blue);
  }

  .pill.active {
    background: var(--color-blue);
    border-color: var(--color-blue);
    color: #fff;
  }

  /* List */
  .list-section {
    max-height: calc(100vh - 200px);
    overflow-y: auto;
  }

  .empty {
    text-align: center;
    padding: 40px 0;
  }

  .empty-title {
    font-size: var(--font-size-base);
    color: var(--color-secondary-label);
    font-weight: 500;
  }

  .empty-desc {
    font-size: var(--font-size-xs);
    color: var(--color-tertiary-label);
    margin-top: 6px;
  }

  .history-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 0;
    gap: 12px;
  }

  .history-content {
    display: flex;
    align-items: baseline;
    gap: 10px;
    min-width: 0;
    flex: 1;
  }

  .history-time {
    font-size: var(--font-size-xs);
    color: var(--color-tertiary-label);
    white-space: nowrap;
    flex-shrink: 0;
    min-width: 40px;
  }

  .history-text {
    font-size: var(--font-size-sm);
    color: var(--color-label);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .history-actions {
    display: flex;
    gap: 4px;
    flex-shrink: 0;
    opacity: 0;
    transition: opacity 0.15s ease;
  }

  .history-row:hover .history-actions {
    opacity: 1;
  }

  .action-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: none;
    background: transparent;
    border-radius: var(--radius-sm);
    color: var(--color-tertiary-label);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .action-btn:hover {
    background: var(--color-bg-secondary);
  }

  .copy-btn:hover {
    color: var(--color-blue);
  }

  .delete-btn:hover {
    color: var(--color-red);
  }

  .divider {
    height: 1px;
    background: var(--color-separator);
  }

  /* Footer */
  .footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    padding: 4px 0;
  }

  .confirm-text {
    font-size: var(--font-size-xs);
    color: var(--color-secondary-label);
    margin-right: auto;
  }

  .clear-btn {
    padding: 4px 0;
    background: transparent;
    border: none;
    font-size: var(--font-size-sm);
    font-weight: 500;
    color: var(--color-red);
    cursor: pointer;
    opacity: 0.8;
    transition: opacity 0.15s ease;
  }

  .clear-btn:hover { opacity: 1; }

  .clear-btn.confirm {
    opacity: 1;
  }

  .cancel-btn {
    padding: 2px 6px;
    background: transparent;
    border: none;
    font-size: var(--font-size-base);
    color: var(--color-tertiary-label);
    cursor: pointer;
  }

  .cancel-btn:hover { color: var(--color-label); }
</style>
