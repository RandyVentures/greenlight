import { useEffect, useRef, useState, useId } from 'react';
import { createRoot } from 'react-dom/client';
import { initial, labels, counts, validate, verdict, sessionToken, checkTitle } from './model.mjs';
import './style.css';

let token = '';
try {
  token = sessionToken(window.location, sessionStorage);
} catch {
  token = window.location.hash.slice(1);
}
if (window.location.hash) history.replaceState(null, '', window.location.pathname);

async function api(path, body) {
  const response = await fetch('/api/' + path, {
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    ...(body ? { method: 'POST', body: JSON.stringify(body) } : {}),
  });
  if (!response.ok) {
    const text = await response.text();
    let message = text;
    try {
      message = JSON.parse(text).error || text;
    } catch {}
    throw new Error(message);
  }
  return response.json();
}

function Icon({ name, size = 20, ...props }) {
  const paths = {
    arrow: <path d="M5 12h14m-5-5 5 5-5 5" />,
    folder: <path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />,
    chevron: <path d="m9 5 7 7-7 7" />,
    check: <path d="m5 12 4 4L19 6" />,
    plus: <path d="M12 5v14M5 12h14" />,
    close: <path d="m6 6 12 12M6 18 18 6" />,
    shield: (
      <>
        <path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6z" />
        <path d="m8 12 3 3 5-6" />
      </>
    ),
    globe: (
      <>
        <circle cx="12" cy="12" r="9" />
        <ellipse cx="12" cy="12" rx="4" ry="9" />
        <path d="M3 12h18" />
      </>
    ),
    user: (
      <>
        <circle cx="12" cy="8" r="4" />
        <path d="M4 21v-3a8 8 0 0 1 16 0v3" />
      </>
    ),
    eye: (
      <>
        <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12" />
        <circle cx="12" cy="12" r="3" />
      </>
    ),
    image: (
      <>
        <rect x="3" y="3" width="18" height="18" rx="3" />
        <circle cx="8" cy="8" r="1" />
        <path d="m3 17 6-6 5 5 3-3 4 4" />
      </>
    ),
    code: (
      <>
        <path d="m7 6-6 6 6 6m10-12 6 6-6 6m-3-16-4 20" />
      </>
    ),
    lock: (
      <>
        <rect x="5" y="10" width="14" height="11" rx="2" />
        <path d="M8 10V7a4 4 0 0 1 8 0v3" />
      </>
    ),
    download: (
      <>
        <path d="M12 3v12m-5-5 5 5 5-5M4 17v4h16v-4" />
      </>
    ),
    circle: <circle cx="12" cy="12" r="8" />,
    alert: (
      <>
        <path d="M12 3 2 21h20zM12 9v5m0 3v1" />
      </>
    ),
    back: <path d="M19 12H5m5-5-5 5 5 5" />,
  };
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      {paths[name] || paths.circle}
    </svg>
  );
}

function Toggle({ label, detail, icon, checked, onChange }) {
  return (
    <label className="feature-row">
      <span className="feature-icon">
        <Icon name={icon} />
      </span>
      <span className="feature-copy">
        <strong>{label}</strong>
        <span>{detail}</span>
      </span>
      <input
        type="checkbox"
        className="switch"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="switch-track" aria-hidden="true" />
    </label>
  );
}

function Field({
  label,
  value,
  onChange,
  onBrowse,
  placeholder = 'Choose a file or folder',
  hint,
  optional = false,
}) {
  const id = useId();
  return (
    <div className="field">
      <label className="field-label" htmlFor={id}>
        {label}
        {optional && <small>Optional</small>}
      </label>
      <span className="path-input">
        <input
          id={id}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          spellCheck="false"
        />
        <button type="button" className="browse" onClick={onBrowse} aria-label={`Browse ${label}`}>
          <Icon name="folder" size={18} />
        </button>
      </span>
      {hint && <span className="hint">{hint}</span>}
    </div>
  );
}

function Picker({ workspace, kind, onSelect, onClose }) {
  const [path, setPath] = useState('.');
  const [data, setData] = useState(null);
  const [error, setError] = useState('');
  const dialog = useRef(null);
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  useEffect(() => {
    let active = true;
    setError('');
    api('browse?path=' + encodeURIComponent(path))
      .then((d) => {
        if (active) setData(d);
      })
      .catch((e) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, [path]);
  const up = () => setPath(path.split('/').slice(0, -1).join('/') || '.');
  return (
    <dialog
      ref={dialog}
      className="picker"
      aria-labelledby="picker-title"
      onCancel={onClose}
      onClick={(e) => {
        if (e.target === dialog.current) onClose();
      }}
    >
      <div className="picker-head">
        <div>
          <span className="eyebrow">LOCAL FILES</span>
          <h2 id="picker-title">
            {kind === 'directory'
              ? 'Choose a folder'
              : kind === 'file'
                ? 'Choose a file'
                : 'Choose a file or folder'}
          </h2>
        </div>
        <button className="icon-button" aria-label="Close file picker" onClick={onClose}>
          <Icon name="close" />
        </button>
      </div>
      <div className="picker-path">
        <button
          className="icon-button"
          onClick={up}
          disabled={path === '.'}
          aria-label="Parent folder"
        >
          <Icon name="back" size={18} />
        </button>
        <span>
          {workspace}
          {path !== '.' ? '/' + path : ''}
        </span>
      </div>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      <div className="file-list">
        {data?.entries
          .filter((e) => kind !== 'directory' || e.directory)
          .map((e) => (
            <button key={e.path} onClick={() => (e.directory ? setPath(e.path) : onSelect(e.path))}>
              <Icon name={e.directory ? 'folder' : 'code'} size={18} />
              <span>{e.name}</span>
              <Icon name={e.directory ? 'chevron' : 'plus'} size={16} />
            </button>
          ))}
        {data?.entries.length === 0 && <p className="muted">This folder is empty.</p>}
      </div>
      <div className="picker-footer">
        <span>Files stay on this computer.</span>
        {kind !== 'file' ? (
          <button className="button primary" onClick={() => onSelect(path)}>
            Use this folder <Icon name="check" size={16} />
          </button>
        ) : (
          <button className="button secondary" onClick={onClose}>
            Cancel
          </button>
        )}
      </div>
    </dialog>
  );
}

function Plan({ steps, busy, onRun, ready }) {
  const done = steps.filter((s) => s.state === 'complete' || s.state === 'error').length;
  return (
    <aside className="plan">
      <div className="plan-top">
        <span className="eyebrow">YOUR PREFLIGHT</span>
        <span className="signal">
          <i />
          <i />
          <i />
        </span>
      </div>
      <div className="plan-number">
        {String(steps.length).padStart(2, '0')}
        <span>check areas</span>
      </div>
      <p className="plan-intro">One checklist, tailored to your app.</p>
      <div className="plan-line" />
      <ol>
        {steps.map((step, index) => (
          <li key={step.id} className={step.state}>
            <span className="step-marker">
              {step.state === 'complete' ? (
                <Icon name="check" size={14} />
              ) : step.state === 'running' ? (
                <span className="spinner" />
              ) : (
                String(index + 1).padStart(2, '0')
              )}
            </span>
            <div>
              <strong>{step.label}</strong>
              <p>{step.detail}</p>
              <span className={`mode ${step.mode}`}>
                {step.mode === 'manual'
                  ? 'Manual review'
                  : step.mode === 'online'
                    ? 'Online · opt-in'
                    : step.mode === 'mixed'
                      ? 'Automated + manual'
                      : 'Automated'}
              </span>
            </div>
          </li>
        ))}
      </ol>
      <button className="button run" onClick={onRun} disabled={busy || !ready}>
        {busy ? (
          <>
            <span className="spinner" /> Running {done}/{steps.length}
          </>
        ) : (
          <>
            Run preflight <Icon name="arrow" />
          </>
        )}
      </button>
      <div className="plan-note">
        <Icon name="lock" size={14} />
        <span>Local by default. No uploads or account required.</span>
      </div>
    </aside>
  );
}

function ResultRow({ row, images }) {
  const [open, setOpen] = useState(false);
  const image = images.find((im) => im.file === row.file);
  return (
    <div className={`result-row ${open ? 'expanded' : ''}`}>
      <button className="result-trigger" onClick={() => setOpen(!open)} aria-expanded={open}>
        <span className={`status-icon ${row.status}`}>
          <Icon
            name={
              row.status === 'passed'
                ? 'check'
                : row.status === 'failed'
                  ? 'close'
                  : row.status === 'suspected'
                    ? 'alert'
                    : 'circle'
            }
            size={16}
          />
        </span>
        <span className="result-name">
          <strong>{checkTitle(row)}</strong>
          <small>{labels[row.group]}</small>
        </span>
        <span className={`status-text ${row.status}`}>
          {row.status === 'suspected'
            ? 'Needs review'
            : row.status === 'failed'
              ? 'Blocker'
              : row.status === 'unverified'
                ? 'Unverified'
                : 'Passed'}
        </span>
        <Icon name="chevron" size={16} className="row-chevron" />
      </button>
      {open && (
        <div className="result-detail">
          <p>{row.evidence}</p>
          {row.file && <code>{row.file}</code>}
          {row.fix && (
            <div className="fix">
              <strong>Suggested next step</strong>
              <p>{row.fix}</p>
            </div>
          )}
          {image && (
            <div className="evidence-image">
              <img src={image.data} alt="Screenshot evidence" />
              {row.regions?.map((b, i) => (
                <span
                  key={i}
                  className="evidence-box"
                  style={{
                    left: (b.left / image.width) * 100 + '%',
                    top: (b.top / image.height) * 100 + '%',
                    width: (b.width / image.width) * 100 + '%',
                    height: (b.height / image.height) * 100 + '%',
                  }}
                />
              ))}
            </div>
          )}
          <div className="detail-foot">
            <span>
              {row.confidence || 'Unassessed'} confidence ·{' '}
              {row.required ? 'Applicable check' : 'Outside automated scope'}
            </span>
            {row.guideline?.startsWith('https://') && (
              <a href={row.guideline} target="_blank" rel="noreferrer">
                Policy source ↗
              </a>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function Results({ job, onBack }) {
  const [filter, setFilter] = useState('all');
  const [exportFormat, setExportFormat] = useState('html');
  const [exportError, setExportError] = useState('');
  const totals = counts(job.rows);
  const filtered = job.rows.filter((row) => filter === 'all' || row.status === filter);
  const download = async () => {
    try {
      setExportError('');
      let content = JSON.stringify(job, null, 2);
      if (exportFormat === 'html') {
        const response = await fetch(`/api/jobs/${job.id}/html`, {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (!response.ok)
          throw new Error('The report is no longer available. Run the audit again.');
        content = await response.text();
      }
      const url = URL.createObjectURL(
        new Blob([content], { type: exportFormat === 'html' ? 'text/html' : 'application/json' }),
      );
      const a = document.createElement('a');
      a.href = url;
      a.download = 'greenlight-report.' + exportFormat;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      setExportError(e.message);
    }
  };

  return (
    <main className="results enter">
      <div className="results-toolbar">
        <button className="text-button" onClick={onBack}>
          <Icon name="back" size={17} /> Edit checklist
        </button>
        <div className="export-controls">
          <select
            aria-label="Report format"
            value={exportFormat}
            onChange={(e) => setExportFormat(e.target.value)}
          >
            <option value="html">HTML</option>
            <option value="json">JSON</option>
          </select>
          <button className="button secondary" onClick={download}>
            <Icon name="download" size={17} /> Export
          </button>
        </div>
      </div>
      {exportError && (
        <p role="alert" className="error">
          {exportError}
        </p>
      )}
      <div className="results-heading">
        <div>
          <span className="eyebrow">PREFLIGHT COMPLETE</span>
          <h1>{verdict(job.summary)}</h1>
          <p>
            {job.summary.checked} of {job.summary.applicable} applicable checks evaluated. Completed
            in {(job.elapsed_ms / 1000).toFixed(1)}s.
          </p>
        </div>
        <div className="completion-mark">
          <Icon
            name={
              totals.failed || totals.suspected
                ? 'eye'
                : job.summary.incomplete
                  ? 'circle'
                  : 'check'
            }
            size={40}
          />
        </div>
      </div>
      <div className="result-tabs" role="group" aria-label="Filter findings">
        {[
          ['all', 'All findings', job.rows.length],
          ['failed', 'Blockers', totals.failed],
          ['suspected', 'Needs review', totals.suspected],
          ['unverified', 'Unverified', totals.unverified],
          ['passed', 'Passed', totals.passed],
        ].map(([key, title, count]) => (
          <button
            key={key}
            className={filter === key ? 'active' : ''}
            onClick={() => setFilter(key)}
          >
            {title}
            <span>{count}</span>
          </button>
        ))}
      </div>
      <div className="result-list">
        {filtered.map((row, index) => (
          <ResultRow key={row.group + row.id + row.file + index} row={row} images={job.images} />
        ))}
        {!filtered.length && (
          <div className="empty-state">
            <Icon name="check" size={30} />
            <h2>Nothing in this category.</h2>
            <p>Other checks may still need your attention.</p>
          </div>
        )}
      </div>
      <details className="scope">
        <summary>
          What this audit covers <Icon name="plus" size={16} />
        </summary>
        <ul>
          {[...new Set(job.limitations)].map((l) => (
            <li key={l}>{l}</li>
          ))}
        </ul>
      </details>
      <p className="review-note">
        <Icon name="shield" size={17} /> Evidence for your review. Approval is always Apple's
        decision.
      </p>
    </main>
  );
}

function App() {
  const [info, setInfo] = useState(null);
  const [selection, setSelection] = useState(initial);
  const [steps, setSteps] = useState([]);
  const [error, setError] = useState('');
  const [picker, setPicker] = useState(null);
  const [job, setJob] = useState(null);
  const [busy, setBusy] = useState(false);
  const [showResults, setShowResults] = useState(false);
  const update = (key, value) => setSelection((s) => ({ ...s, [key]: value }));
  useEffect(() => {
    api('info')
      .then(setInfo)
      .catch((e) => setError(e.message));
  }, []);
  useEffect(() => {
    if (busy) return;
    let active = true;
    const timer = setTimeout(
      () =>
        api('plan', selection)
          .then((p) => {
            if (active) setSteps(p);
          })
          .catch((e) => {
            if (active) setError(e.message);
          }),
      100,
    );
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [selection, busy]);
  useEffect(() => {
    if (!busy || !job?.id) return;
    let active = true;
    let timer;
    const poll = async () => {
      try {
        const next = await api('jobs/' + job.id);
        if (!active) return;
        setJob(next);
        setSteps(next.steps);
        if (next.status === 'complete') {
          setBusy(false);
          setShowResults(true);
          window.scrollTo({ top: 0, behavior: 'smooth' });
        } else timer = setTimeout(poll, 450);
      } catch (e) {
        if (active) {
          setError(e.message);
          setBusy(false);
        }
      }
    };
    poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [busy, job?.id]);
  const run = async () => {
    const issue = validate(selection);
    if (issue) {
      setError(issue);
      return;
    }
    setError('');
    setJob(null);
    setBusy(true);
    try {
      const started = await api('run', selection);
      setJob({ id: started.id });
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  };
  const browse = (key, kind = 'any') =>
    setPicker({
      kind,
      select: (path) => {
        update(key, path);
        setPicker(null);
      },
    });
  const setPayment = (value) =>
    setSelection((s) => ({
      ...s,
      payments: value,
      products:
        value === 'none'
          ? s.products
          : s.products.length
            ? s.products
            : [{ id: '', entitlement: '' }],
    }));
  const loadExample = () => {
    setError('');
    setSelection({
      ...initial,
      project: 'examples/ui-demo',
      screenshots: true,
      screenshot_path: 'examples/screenshots',
      display_type: 'APP_IPHONE_47',
      payments: 'revenuecat',
      products: [{ id: 'com.example.pro', entitlement: 'pro' }],
      source_files: ['examples/purchases/Products.swift'],
      storekit_file: 'examples/purchases/Local.storekit',
      asc_products_file: 'examples/purchases/asc-products.json',
      revenuecat_file: 'examples/purchases/revenuecat-products.json',
    });
  };
  return (
    <>
      <header className="topbar">
        <a
          href="/"
          className="brand"
          onClick={(e) => {
            e.preventDefault();
            setShowResults(false);
          }}
        >
          <span className="brand-mark">
            <i />
          </span>
          greenlight<span className="fork-tag">WORKSPACE</span>
        </a>
        <div className="topbar-right">
          <span className="local-status">
            <i />
            Local session
          </span>
          <a
            className="github-link"
            href="https://github.com/RandyVentures/greenlight"
            target="_blank"
            rel="noreferrer"
          >
            GitHub ↗
          </a>
        </div>
      </header>
      {showResults && job?.status === 'complete' ? (
        <Results
          job={job}
          onBack={() => {
            setShowResults(false);
            setSteps(steps.map((s) => ({ ...s, state: 'queued' })));
          }}
        />
      ) : (
        <main className="workspace enter">
          <div className="workspace-heading">
            <div>
              <span className="eyebrow">APP STORE READINESS</span>
              <h1>
                Build your checklist<span className="green-dot">.</span>
              </h1>
              <p>Choose what your app uses. Check the evidence before you submit.</p>
            </div>
            <button
              className="text-button example-button"
              onClick={loadExample}
              disabled={busy || !info?.example_available}
              title={
                info?.example_available
                  ? 'Run the included synthetic example'
                  : 'Open Greenlight’s repository as the workspace to try its example'
              }
            >
              <Icon name="plus" size={16} /> Try an example
            </button>
          </div>
          {error && (
            <div className="error-banner" role="alert">
              <Icon name="alert" size={18} />
              <span>{error}</span>
              <button
                className="icon-button"
                onClick={() => setError('')}
                aria-label="Dismiss error"
              >
                <Icon name="close" size={16} />
              </button>
            </div>
          )}
          <div className="workspace-grid">
            <fieldset className="setup" disabled={busy}>
              <section className="setup-section">
                <div className="section-heading">
                  <span className="section-number">01</span>
                  <h2>The app</h2>
                </div>
                <Field
                  label="Project folder"
                  value={selection.project}
                  onChange={(v) => update('project', v)}
                  onBrowse={() => browse('project', 'directory')}
                  hint={info ? `Workspace: ${info.workspace}` : 'Opening your local workspace…'}
                />
                <div className="field">
                  <span className="field-label">Platform</span>
                  <div className="platforms">
                    {[
                      ['ios', 'iOS & iPadOS'],
                      ['macos', 'macOS'],
                    ].map(([value, text]) => (
                      <label key={value} className={selection.platform === value ? 'selected' : ''}>
                        <input
                          type="radio"
                          name="platform"
                          value={value}
                          checked={selection.platform === value}
                          onChange={() => {
                            update('platform', value);
                            update(
                              'display_type',
                              value === 'macos' ? 'APP_DESKTOP' : 'APP_IPHONE_67',
                            );
                          }}
                        />
                        <Icon name={value === 'ios' ? 'image' : 'code'} size={18} />
                        {text}
                        {selection.platform === value && (
                          <Icon name="check" size={16} className="platform-check" />
                        )}
                      </label>
                    ))}
                  </div>
                </div>
              </section>
              <section className="setup-section">
                <div className="section-heading">
                  <span className="section-number">02</span>
                  <h2>What’s inside</h2>
                </div>
                <label className="field">
                  <span className="field-label">In-app purchases</span>
                  <select value={selection.payments} onChange={(e) => setPayment(e.target.value)}>
                    <option value="none">No in-app purchases</option>
                    <option value="storekit">StoreKit · direct integration</option>
                    <option value="revenuecat">RevenueCat</option>
                  </select>
                </label>
                <div className="features">
                  <Toggle
                    label="User accounts"
                    detail="Sign-in, account creation, and deletion"
                    icon="user"
                    checked={selection.accounts}
                    onChange={(v) => update('accounts', v)}
                  />
                  <Toggle
                    label="Tracking & advertising"
                    detail="Consent prompts and privacy disclosures"
                    icon="eye"
                    checked={selection.tracking}
                    onChange={(v) => update('tracking', v)}
                  />
                  <Toggle
                    label="Multiple languages"
                    detail="Locale coverage and translation review"
                    icon="globe"
                    checked={selection.localized}
                    onChange={(v) => update('localized', v)}
                  />
                </div>
                {selection.localized && (
                  <label className="field reveal">
                    <span className="field-label">Expected locales</span>
                    <input
                      placeholder="en-US, es-MX, pt-BR"
                      defaultValue={selection.locales.join(', ')}
                      onBlur={(e) =>
                        update(
                          'locales',
                          e.target.value
                            .split(',')
                            .map((v) => v.trim())
                            .filter(Boolean),
                        )
                      }
                    />
                    <span className="hint">
                      Separate locale codes with commas. Screenshot folders should use these names.
                    </span>
                  </label>
                )}
              </section>
              <section className="setup-section">
                <div className="section-heading">
                  <span className="section-number">03</span>
                  <h2>The evidence</h2>
                  <span className="section-aside">READ ONLY</span>
                </div>
                <Toggle
                  label="Audit screenshots"
                  detail="Size checks and local text recognition"
                  icon="image"
                  checked={selection.screenshots}
                  onChange={(v) => update('screenshots', v)}
                />
                {selection.screenshots && (
                  <div className="conditional reveal">
                    <Field
                      label="Screenshot folder or file"
                      value={selection.screenshot_path}
                      onChange={(v) => update('screenshot_path', v)}
                      onBrowse={() => browse('screenshot_path')}
                    />
                    <div className="field-pair">
                      <label className="field">
                        <span className="field-label">Display size</span>
                        <select
                          value={selection.display_type}
                          onChange={(e) => update('display_type', e.target.value)}
                        >
                          <option value="APP_IPHONE_67">iPhone · large Dynamic Island</option>
                          <option value="APP_IPHONE_65">iPhone · large Face ID</option>
                          <option value="APP_IPHONE_61">iPhone · medium Dynamic Island</option>
                          <option value="APP_IPHONE_47">iPhone · 4.7-inch</option>
                          <option value="APP_IPAD_PRO_3GEN_129">iPad · 13-inch</option>
                          <option value="APP_DESKTOP">Mac</option>
                          <option value="">Dimensions unverified</option>
                        </select>
                      </label>
                      <label className="field">
                        <span className="field-label">OCR language</span>
                        <select
                          value={selection.language}
                          onChange={(e) => update('language', e.target.value)}
                        >
                          <option value="eng">English</option>
                          <option value="spa">Spanish</option>
                          <option value="eng+spa">English + Spanish</option>
                          <option value="por">Portuguese</option>
                          <option value="fra">French</option>
                          <option value="deu">German</option>
                        </select>
                      </label>
                    </div>
                    <p className="hint">
                      Uses Tesseract installed on this computer. One display size per run.
                    </p>
                  </div>
                )}
                {selection.payments !== 'none' && (
                  <div className="conditional purchases reveal">
                    <div className="subheading">
                      <h3>Expected products</h3>
                      <button
                        type="button"
                        className="text-button"
                        onClick={() =>
                          update('products', [...selection.products, { id: '', entitlement: '' }])
                        }
                      >
                        <Icon name="plus" size={15} /> Add product
                      </button>
                    </div>
                    {selection.products.map((p, index) => (
                      <div className="product-row" key={index}>
                        <label className="field">
                          <span className="field-label">Product ID</span>
                          <input
                            placeholder="com.example.pro"
                            value={p.id}
                            onChange={(e) =>
                              update(
                                'products',
                                selection.products.map((item, i) =>
                                  i === index ? { ...item, id: e.target.value } : item,
                                ),
                              )
                            }
                          />
                        </label>
                        {selection.payments === 'revenuecat' && (
                          <label className="field">
                            <span className="field-label">Entitlement</span>
                            <input
                              placeholder="pro"
                              value={p.entitlement}
                              onChange={(e) =>
                                update(
                                  'products',
                                  selection.products.map((item, i) =>
                                    i === index ? { ...item, entitlement: e.target.value } : item,
                                  ),
                                )
                              }
                            />
                          </label>
                        )}
                        <button
                          type="button"
                          className="icon-button remove-product"
                          onClick={() =>
                            update(
                              'products',
                              selection.products.filter((_, i) => i !== index),
                            )
                          }
                          aria-label={`Remove product ${index + 1}`}
                        >
                          <Icon name="close" size={16} />
                        </button>
                      </div>
                    ))}
                    <div className="field">
                      <span className="field-label">Source files</span>
                      {selection.source_files.map((file) => (
                        <div key={file} className="selected-file">
                          <Icon name="code" size={16} />
                          <span>{file}</span>
                          <button
                            className="icon-button"
                            aria-label={`Remove ${file}`}
                            onClick={() =>
                              update(
                                'source_files',
                                selection.source_files.filter((p) => p !== file),
                              )
                            }
                          >
                            <Icon name="close" size={14} />
                          </button>
                        </div>
                      ))}
                      <button
                        type="button"
                        className="button secondary add-file"
                        onClick={() =>
                          setPicker({
                            kind: 'file',
                            select: (path) => {
                              update('source_files', [
                                ...new Set([...selection.source_files, path]),
                              ]);
                              setPicker(null);
                            },
                          })
                        }
                      >
                        <Icon name="plus" size={16} /> Choose source file
                      </button>
                    </div>
                    <Field
                      label="StoreKit configuration"
                      value={selection.storekit_file}
                      onChange={(v) => update('storekit_file', v)}
                      onBrowse={() => browse('storekit_file', 'file')}
                      optional
                    />
                    <Field
                      label="App Store product catalog"
                      value={selection.asc_products_file}
                      onChange={(v) => update('asc_products_file', v)}
                      onBrowse={() => browse('asc_products_file', 'file')}
                      placeholder="Choose an ASC product JSON snapshot"
                    />
                    {selection.payments === 'revenuecat' && (
                      <Field
                        label="RevenueCat catalog"
                        value={selection.revenuecat_file}
                        onChange={(v) => update('revenuecat_file', v)}
                        onBrowse={() => browse('revenuecat_file', 'file')}
                        placeholder="Choose a normalized catalog JSON snapshot"
                      />
                    )}
                    <p className="hint">
                      Missing catalogs stay unverified. Purchase and restore flows still need device
                      testing.
                    </p>
                  </div>
                )}
              </section>
              <details className="advanced">
                <summary>
                  App Store Connect{' '}
                  <span>
                    Optional online check <Icon name="plus" size={16} />
                  </span>
                </summary>
                <div className="advanced-content">
                  <Toggle
                    label="Read App Store Connect"
                    detail="Uses your saved API key. No changes or submissions."
                    icon="shield"
                    checked={selection.live_asc}
                    onChange={(v) => update('live_asc', v)}
                  />
                  {selection.live_asc && (
                    <div className="reveal">
                      <div className="field-pair">
                        <label className="field">
                          <span className="field-label">App ID</span>
                          <input
                            value={selection.app_id}
                            onChange={(e) => update('app_id', e.target.value)}
                            placeholder="Apple app ID"
                          />
                        </label>
                        <label className="field">
                          <span className="field-label">Expected build</span>
                          <input
                            value={selection.build}
                            onChange={(e) => update('build', e.target.value)}
                            placeholder="Optional"
                          />
                        </label>
                      </div>
                      <label className="field">
                        <span className="field-label">Version resource ID</span>
                        <input
                          value={selection.version_id}
                          onChange={(e) => update('version_id', e.target.value)}
                          placeholder="App Store version resource ID"
                        />
                      </label>
                      <p className="hint">
                        Configure credentials with greenlight auth setup. Keys never enter the
                        browser.
                      </p>
                    </div>
                  )}
                </div>
              </details>
            </fieldset>
            <Plan steps={steps} busy={busy} ready={!!info} onRun={run} />
          </div>
        </main>
      )}
      {picker && (
        <Picker
          workspace={info?.name || 'Workspace'}
          kind={picker.kind}
          onSelect={picker.select}
          onClose={() => setPicker(null)}
        />
      )}
      <footer className="footer">
        <span>Built in the open. Kept on your machine.</span>
        <span>
          RandyVentures <span className="footer-divider">/</span> Greenlight · MIT
        </span>
      </footer>
    </>
  );
}

createRoot(document.getElementById('root')).render(<App />);
