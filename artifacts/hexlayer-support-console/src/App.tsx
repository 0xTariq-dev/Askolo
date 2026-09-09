import { useEffect, useRef, useState, type FormEvent } from 'react';
import {
  ArrowUpRight,
  BookOpen,
  Check,
  CheckCircle2,
  ChevronRight,
  Circle,
  Clock3,
  Copy,
  Github,
  Globe2,
  Mail,
  MessageSquare,
  Terminal,
  Zap,
} from 'lucide-react';

type ChannelId = 'sales' | 'support' | 'docs' | 'community';
type FormState = { name: string; email: string; company: string; message: string };
type ErrorState = Partial<Record<keyof FormState, string>>;

const channels: Array<{
  id: ChannelId;
  icon: typeof Zap;
  label: string;
  desc: string;
  meta: string;
  topic: string;
}> = [
  { id: 'sales', icon: Zap, label: 'Talk to sales', desc: 'Enterprise plans, volume pricing, security reviews & procurement.', meta: 'Replies in < 4 hrs', topic: 'Sales inquiry' },
  { id: 'support', icon: MessageSquare, label: 'Engineering support', desc: 'Build failures, SDK bugs, runtime issues. Routed straight to the team.', meta: 'Median response 38 min', topic: 'Bug report' },
  { id: 'docs', icon: BookOpen, label: 'Documentation', desc: 'Guides, API reference, migration paths and changelogs.', meta: 'docs.hexlayer.dev', topic: 'API access' },
  { id: 'community', icon: Github, label: 'Community', desc: '14,200 developers in Discord & GitHub Discussions. Office hours every Thursday.', meta: 'discord.gg/hexlayer', topic: 'Partnership' },
];
const topics = ['Sales inquiry', 'Bug report', 'API access', 'Partnership', 'Security', 'Other'];
const emptyForm: FormState = { name: '', email: '', company: '', message: '' };

function formatUtc(date: Date) {
  return date.toISOString().slice(11, 19);
}

function Home() {
  const [topic, setTopic] = useState('Sales inquiry');
  const [form, setForm] = useState<FormState>(emptyForm);
  const [errors, setErrors] = useState<ErrorState>({});
  const [submitted, setSubmitted] = useState(false);
  const [copied, setCopied] = useState(false);
  const [time, setTime] = useState(() => new Date());
  const [selectedChannel, setSelectedChannel] = useState<ChannelId | null>(null);
  const [sending, setSending] = useState(false);
  const formRef = useRef<HTMLDivElement>(null);
  const firstErrorRef = useRef<HTMLInputElement | HTMLTextAreaElement | null>(null);

  useEffect(() => {
    const timer = window.setInterval(() => setTime(new Date()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const focusForm = (channel: typeof channels[number]) => {
    setSelectedChannel(channel.id);
    setTopic(channel.topic);
    window.requestAnimationFrame(() => {
      formRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      window.setTimeout(() => document.getElementById('name')?.focus(), 450);
    });
  };

  const updateField = (field: keyof FormState, value: string) => {
    setForm((current) => ({ ...current, [field]: value }));
    if (errors[field]) setErrors((current) => ({ ...current, [field]: undefined }));
  };

  const validate = () => {
    const next: ErrorState = {};
    if (!form.name.trim()) next.name = 'name is required';
    if (!form.email.trim()) next.email = 'work_email is required';
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email)) next.email = 'enter a valid email address';
    if (!form.message.trim()) next.message = 'message is required';
    setErrors(next);
    return next;
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const nextErrors = validate();
    if (Object.keys(nextErrors).length > 0) {
      const first = Object.keys(nextErrors)[0] as keyof FormState;
      firstErrorRef.current = document.getElementById(first) as HTMLInputElement | HTMLTextAreaElement;
      firstErrorRef.current?.focus();
      return;
    }
    setSending(true);
    window.setTimeout(() => {
      setSending(false);
      setSubmitted(true);
    }, 500);
  };

  const copyEmail = async () => {
    try {
      await navigator.clipboard.writeText('hello@hexlayer.dev');
    } catch {
      // Clipboard permissions are optional; the visible feedback still confirms the action.
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2200);
  };

  const startAgain = () => {
    setSubmitted(false);
    setForm(emptyForm);
    setErrors({});
    setSelectedChannel(null);
    window.requestAnimationFrame(() => document.getElementById('name')?.focus());
  };

  return (
    <div className="console-shell">
      <div className="console-content">
        <header className="topbar" id="top">
          <div className="topbar-inner">
            <a className="brand" href="#top" data-testid="link-brand" aria-label="Hexlayer home">
              <span className="brand-mark"><Terminal size={14} strokeWidth={2.6} aria-hidden="true" /></span>
              <span className="brand-name">hexlayer</span>
              <span className="version">v3.2.1</span>
            </a>
            <nav className="nav" aria-label="Primary navigation">
              <a href="#channels" data-testid="link-docs">/docs</a>
              <a href="#contact-form" data-testid="link-pricing">/pricing</a>
              <a href="#footer" data-testid="link-changelog">/changelog</a>
              <a href="#contact-form" aria-current="page" data-testid="link-contact"><span className="status-dot" aria-hidden="true" />/contact</a>
            </nav>
            <div className="utc" data-testid="status-utc"><Clock3 size={11} aria-hidden="true" /> {formatUtc(time)} UTC</div>
          </div>
        </header>

        <main className="main">
          <section className="hero" aria-labelledby="page-title">
            <div>
              <p className="eyebrow"><span className="prompt-prefix">$</span> hexlayer contact --human <span className="cursor" aria-hidden="true" /></p>
              <h1 id="page-title">Talk to the people<br /><span className="headline-muted">who ship the tool.</span></h1>
              <p className="hero-copy">No ticket queues routed offshore. Every message lands with an engineer on the team that builds Hexlayer — usually the one who wrote the code you're asking about.</p>
            </div>
            <aside className="panel system-panel" aria-label="Hexlayer system status">
              <div className="panel-heading">
                <span className="micro-label">System status</span>
                <span className="live"><span className="status-dot" aria-hidden="true" /> All operational</span>
              </div>
              {[['Build pipeline', '99.99%'], ['Edge runtime', '99.97%'], ['Registry API', '100.0%'], ['Dashboard', '99.95%']].map(([name, value]) => (
                <div className="metric" key={name} data-testid={`status-${name.toLowerCase().replace(' ', '-')}`}><span>{name}</span><strong>{value}</strong></div>
              ))}
              <a className="status-link mono" href="#footer" data-testid="link-status">status.hexlayer.dev <ArrowUpRight size={12} aria-hidden="true" /></a>
            </aside>
          </section>

          <div className="content-grid">
            <section id="channels" aria-labelledby="channels-title">
              <p className="section-kicker micro-label mono" id="channels-title">// Pick a channel</p>
              <div className="channel-list">
                {channels.map((channel) => {
                  const Icon = channel.icon;
                  const active = selectedChannel === channel.id;
                  return (
                    <button className={`channel${active ? ' selected' : ''}`} type="button" key={channel.id} onClick={() => focusForm(channel)} aria-pressed={active} data-testid={`button-channel-${channel.id}`}>
                      <span className="channel-icon"><Icon size={17} strokeWidth={1.7} aria-hidden="true" /></span>
                      <span className="channel-body">
                        <span className="channel-title"><span>{channel.label}</span><ArrowUpRight className="channel-arrow" size={15} aria-hidden="true" /></span>
                        <span className="channel-desc">{channel.desc}</span>
                        <span className="channel-meta mono">{channel.meta}</span>
                      </span>
                    </button>
                  );
                })}
              </div>
              <div className="direct-email" data-testid="display-direct-email">
                <div><p className="direct-label mono">Prefer plain email?</p><p className="email-address mono">hello@hexlayer.dev</p></div>
                <button className="copy-button" type="button" onClick={copyEmail} data-testid="button-copy-email" aria-label="Copy hello@hexlayer.dev email address">
                  {copied ? <Check size={13} aria-hidden="true" /> : <Copy size={13} aria-hidden="true" />}{copied ? 'copied' : 'copy'}
                </button>
              </div>
              <div className="office-grid" aria-label="Hexlayer offices">
                <div><p className="office-label mono"><Globe2 size={11} aria-hidden="true" /> SAN FRANCISCO</p><p className="office-address mono">580 Howard St, Floor 4<br />CA 94105 · GMT-8</p></div>
                <div><p className="office-label mono"><Globe2 size={11} aria-hidden="true" /> AMSTERDAM</p><p className="office-address mono">Herengracht 182<br />1016 BR · GMT+1</p></div>
              </div>
            </section>

            <section id="contact-form" ref={formRef} className="terminal-card panel" aria-labelledby="form-title">
              <div className="terminal-bar">
                <div className="terminal-dots" aria-hidden="true"><span /><span /><span /></div>
                <span className="terminal-title" id="form-title">~/contact/new-message</span>
                <span className="terminal-encoding">utf-8</span>
              </div>
              {!submitted ? (
                <form className="form-content" onSubmit={handleSubmit} noValidate>
                  <fieldset className="form-section">
                    <legend className="form-heading">01 — What's this about?</legend>
                    <div className="topic-list">
                      {topics.map((item) => (
                        <button className={`topic${topic === item ? ' active' : ''}`} type="button" key={item} onClick={() => setTopic(item)} aria-pressed={topic === item} data-testid={`button-topic-${item.toLowerCase().replace(' ', '-')}`}>
                          {topic === item ? <CheckCircle2 size={11} aria-hidden="true" /> : <Circle size={11} aria-hidden="true" />}{item}
                        </button>
                      ))}
                    </div>
                  </fieldset>
                  <fieldset className="form-section">
                    <legend className="form-heading">02 — Who are you?</legend>
                    <div className="field-grid">
                      <div>
                        <label className={`field${errors.name ? ' invalid' : ''}`} htmlFor="name"><span className="field-label">name</span><input id="name" name="name" value={form.name} onChange={(event) => updateField('name', event.target.value)} placeholder="Ada Lovelace" aria-invalid={Boolean(errors.name)} aria-describedby={errors.name ? 'name-error' : undefined} data-testid="input-name" /></label>
                        {errors.name && <span className="error-message" id="name-error" role="alert">{errors.name}</span>}
                      </div>
                      <div>
                        <label className={`field${errors.email ? ' invalid' : ''}`} htmlFor="email"><span className="field-label">work_email</span><input id="email" name="email" type="email" value={form.email} onChange={(event) => updateField('email', event.target.value)} placeholder="ada@analytical.engine" aria-invalid={Boolean(errors.email)} aria-describedby={errors.email ? 'email-error' : undefined} data-testid="input-email" /></label>
                        {errors.email && <span className="error-message" id="email-error" role="alert">{errors.email}</span>}
                      </div>
                    </div>
                    <label className="field" htmlFor="company" style={{ marginTop: 12 }}><span className="field-label">company <span className="optional">(optional)</span></span><input id="company" name="company" value={form.company} onChange={(event) => updateField('company', event.target.value)} placeholder="Acme Robotics — 40 engineers" data-testid="input-company" /></label>
                  </fieldset>
                  <fieldset className="form-section">
                    <legend className="form-heading">03 — The details</legend>
                    <label className={`field${errors.message ? ' invalid' : ''}`} htmlFor="message"><span className="field-label">message</span><textarea id="message" name="message" rows={5} value={form.message} onChange={(event) => updateField('message', event.target.value)} placeholder="We're migrating 200+ services to Hexlayer's build cache and need to understand SSO + audit log support before procurement signs off…" aria-invalid={Boolean(errors.message)} aria-describedby={errors.message ? 'message-error' : undefined} data-testid="input-message" /></label>
                    {errors.message && <span className="error-message" id="message-error" role="alert">{errors.message}</span>}
                  </fieldset>
                  <div className="form-footer">
                    <p className="form-note">Routed by topic · No marketing follow-ups<br />Avg first reply: <strong>2h 14m</strong></p>
                    <button className="submit-button" type="submit" disabled={sending} data-testid="button-submit-message">{sending ? 'routing…' : 'send_message()'} <ChevronRight size={15} aria-hidden="true" /></button>
                  </div>
                </form>
              ) : (
                <div className="result" role="status" aria-live="polite" data-testid="status-submission-success">
                  <p className="result-command">$ hexlayer contact send --topic "{topic.toLowerCase()}"</p>
                  <p className="result-line">→ validating payload <span className="result-check">✓</span></p>
                  <p className="result-line">→ routing to {topic === 'Sales inquiry' ? 'sales-eng' : 'core-runtime'} on-call <span className="result-check">✓</span></p>
                  <p className="result-line">→ confirmation sent to <span className="result-email">{form.email}</span> <span className="result-check">✓</span></p>
                  <div className="success-box">
                    <p className="success-title"><CheckCircle2 size={16} aria-hidden="true" /> Message #HX-48217 created</p>
                    <p className="success-copy">Thanks, {form.name.split(' ')[0] || 'there'}. An engineer will reply within a few hours (Mon–Fri, SF & Amsterdam hours). Urgent production issue? Ping us in Discord — it pages on-call.</p>
                  </div>
                  <button className="another-button" type="button" onClick={startAgain} data-testid="button-send-another">↺ send another message</button>
                </div>
              )}
            </section>
          </div>
          <div className="trust" data-testid="display-trust-strip"><span>SOC 2 Type II</span><span className="trust-separator" /><span>GDPR / DPA on request</span><span className="trust-separator" /><span>Trusted by Vercel, Ramp, Linear & 9,000+ teams</span></div>
        </main>

        <footer className="footer" id="footer">
          <div className="footer-inner">
            <span>© 2025 Hexlayer Systems, Inc.</span>
            <div className="footer-links">
              <a className="footer-link" href="https://github.com/hexlayer" data-testid="link-github"><Github size={12} aria-hidden="true" /> github</a>
              <a className="footer-link" href="mailto:hello@hexlayer.dev" data-testid="link-email"><Mail size={12} aria-hidden="true" /> hello@hexlayer.dev</a>
            </div>
          </div>
        </footer>
      </div>
    </div>
  );
}

function App() {
  return <Home />;
}

export default App;