import { useEffect, useRef, useState } from 'react';

const rules = [
  ['', 'Manual prevention reminder'],
  ['pricing-copy', 'Screenshot pricing / trial copy'],
  ['dimensions', 'Screenshot dimensions'],
  ['alpha-channel', 'Image transparency'],
  ['source-product-id', 'Product IDs in source'],
  ['asc-product-state', 'App Store product status'],
  ['revenuecat-entitlement', 'RevenueCat entitlement mapping'],
  ['locale-screenshots', 'Screenshot locale coverage'],
  ['release-packaging', 'Packaged fixtures or credential files'],
  ['release-extension-version', 'Extension version / build'],
  ['metadata-review-notes', 'Review notes'],
  ['human.submission', 'Review package and access'],
  ['human.purchases', 'Purchase lifecycle acceptance'],
  ['human.subscriptions', 'Subscription acceptance'],
  ['human.rights', 'Content and service rights'],
  ['human.sync', 'Production sync acceptance'],
];

export function HistoryDialog({ project, api, onClose }) {
  const dialog = useRef(null);
  const [records, setRecords] = useState([]);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [fixes, setFixes] = useState({});
  const empty = {
    origin: 'apple_review',
    guideline: '',
    summary: '',
    root_cause: '',
    prevention: '',
    rule_id: '',
    version: '',
    build: '',
    rejected_at: '',
  };
  const [draft, setDraft] = useState(empty);
  useEffect(() => {
    dialog.current.showModal();
    api('history?project=' + encodeURIComponent(project))
      .then(setRecords)
      .catch((e) => setError(e.message));
  }, [project]);
  const update = (key, value) => setDraft((d) => ({ ...d, [key]: value }));
  const save = async (e) => {
    e.preventDefault();
    setSaving(true);
    setError('');
    try {
      setRecords(await api('history', { project, action: 'add', record: draft }));
      setDraft(empty);
    } catch (e) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };
  const resolve = async (id) => {
    setSaving(true);
    setError('');
    try {
      setRecords(
        await api('history', { project, action: 'resolve', id, resolution: fixes[id] || '' }),
      );
    } catch (e) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };
  return (
    <dialog
      ref={dialog}
      className="picker history-dialog"
      aria-labelledby="history-title"
      onCancel={onClose}
    >
      <div className="picker-head">
        <div>
          <span className="eyebrow">PRIVATE · LOCAL HISTORY</span>
          <h2 id="history-title">Learn from the last review.</h2>
        </div>
        <button className="icon-button" onClick={onClose} aria-label="Close rejection history">
          ×
        </button>
      </div>
      <p className="hint history-intro">
        Project: {project}. Saved outside your app repository. Resolved issues are still checked on
        future audits.
      </p>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      <div className="history-scroll">
        <section>
          <h3>Record an issue</h3>
          <form onSubmit={save}>
            <div className="field-pair">
              <label className="field">
                <span className="field-label">Source</span>
                <select value={draft.origin} onChange={(e) => update('origin', e.target.value)}>
                  <option value="apple_review">Apple review rejection</option>
                  <option value="internal_preflight">Internal preflight finding</option>
                </select>
              </label>
              <label className="field">
                <span className="field-label">Guideline</span>
                <input
                  placeholder="e.g. 2.3.7"
                  value={draft.guideline}
                  onChange={(e) => update('guideline', e.target.value)}
                  maxLength={100}
                />
              </label>
            </div>
            <div className="field-pair">
              <label className="field">
                <span className="field-label">Version / build</span>
                <div className="field-pair">
                  <input
                    aria-label="Reviewed version"
                    placeholder="Version"
                    value={draft.version}
                    onChange={(e) => update('version', e.target.value)}
                    maxLength={100}
                  />
                  <input
                    aria-label="Reviewed build"
                    placeholder="Build"
                    value={draft.build}
                    onChange={(e) => update('build', e.target.value)}
                    maxLength={100}
                  />
                </div>
              </label>
              <label className="field">
                <span className="field-label">Review date</span>
                <input
                  type="date"
                  value={draft.rejected_at}
                  onChange={(e) => update('rejected_at', e.target.value)}
                />
              </label>
            </div>
            <label className="field">
              <span className="field-label">What happened?</span>
              <input
                required
                maxLength={1000}
                value={draft.summary}
                onChange={(e) => update('summary', e.target.value)}
                placeholder="Briefly describe the rejection or finding"
              />
            </label>
            <label className="field">
              <span className="field-label">Root cause</span>
              <textarea
                maxLength={8000}
                value={draft.root_cause}
                onChange={(e) => update('root_cause', e.target.value)}
                placeholder="Why did this slip through?"
              />
            </label>
            <label className="field">
              <span className="field-label">Check this next time</span>
              <textarea
                required
                maxLength={8000}
                value={draft.prevention}
                onChange={(e) => update('prevention', e.target.value)}
                placeholder="Concrete prevention and retest steps"
              />
            </label>
            <label className="field">
              <span className="field-label">Link a repeat check</span>
              <select
                value={rules.some(([id]) => id === draft.rule_id) ? draft.rule_id : ''}
                onChange={(e) => update('rule_id', e.target.value)}
              >
                {rules.map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
              <span className="hint">
                If the linked check is skipped or lacks evidence, the reminder stays unverified.
              </span>
            </label>
            <label className="field">
              <span className="field-label">
                Exact check ID <small>Optional override</small>
              </span>
              <input
                value={draft.rule_id}
                maxLength={200}
                onChange={(e) => update('rule_id', e.target.value)}
                placeholder="e.g. human.custom.share-sheet"
              />
              <span className="hint">
                Use an existing check ID from a current JSON report, or leave blank for a manual
                reminder.
              </span>
            </label>
            <button className="button primary" type="submit" disabled={saving}>
              Save locally
            </button>
          </form>
        </section>
        <section className="history-records">
          <h3>
            {records.length} recorded {records.length === 1 ? 'issue' : 'issues'}
          </h3>
          {!records.length && (
            <p className="hint">
              No records yet. Internal findings and actual Apple rejections are tracked separately;
              nothing is automatically imported or invented.
            </p>
          )}
          {records.map((r) => (
            <article className="history-record" key={r.id}>
              <div className="subheading">
                <strong>{r.summary}</strong>
                <span className="mode">{r.status}</span>
              </div>
              <small>
                {r.origin === 'apple_review' ? 'Apple review' : 'Internal finding'}
                {r.version ? ` · ${r.version} (${r.build || 'build unspecified'})` : ''}
                {r.rejected_at ? ` · Reviewed ${r.rejected_at}` : ''}
                {r.guideline ? ` · §${r.guideline}` : ''} ·{' '}
                {new Date(r.created_at).toLocaleDateString()}
              </small>
              {r.root_cause && (
                <p>
                  <strong>Cause:</strong> {r.root_cause}
                </p>
              )}
              <p>
                <strong>Prevention:</strong> {r.prevention}
              </p>
              <p className="hint">Repeat check: {r.rule_id || 'Manual review'}</p>
              {r.resolution && (
                <p>
                  <strong>Fix:</strong> {r.resolution}
                </p>
              )}
              {r.status === 'open' && (
                <div className="resolve-row">
                  <label className="field">
                    <span className="field-label">Fix applied</span>
                    <input
                      aria-label={`Fix applied for ${r.summary}`}
                      value={fixes[r.id] || ''}
                      onChange={(e) => setFixes((f) => ({ ...f, [r.id]: e.target.value }))}
                      placeholder="Record the fix before resolving"
                    />
                  </label>
                  <button
                    className="button secondary"
                    onClick={() => resolve(r.id)}
                    disabled={saving || !fixes[r.id]?.trim()}
                  >
                    Mark resolved
                  </button>
                </div>
              )}
            </article>
          ))}
        </section>
      </div>
    </dialog>
  );
}
