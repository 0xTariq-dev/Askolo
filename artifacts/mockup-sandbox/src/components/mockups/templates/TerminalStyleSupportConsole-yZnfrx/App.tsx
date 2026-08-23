import { useState, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import {
  Terminal, ArrowUpRight, Mail, MessageSquare, BookOpen,
  Github, CheckCircle2, Circle, ChevronRight, Copy, Check,
  Zap, Globe, Clock
} from 'lucide-react';

const channels = [
  {
    id: 'sales',
    icon: Zap,
    label: 'Talk to sales',
    desc: 'Enterprise plans, volume pricing, security reviews & procurement.',
    meta: 'Replies in < 4 hrs',
    href: 'sales@hexlayer.dev',
  },
  {
    id: 'support',
    icon: MessageSquare,
    label: 'Engineering support',
    desc: 'Build failures, SDK bugs, runtime issues. Routed straight to the team.',
    meta: 'Median response 38 min',
    href: 'support@hexlayer.dev',
  },
  {
    id: 'docs',
    icon: BookOpen,
    label: 'Documentation',
    desc: 'Guides, API reference, migration paths and changelogs.',
    meta: 'docs.hexlayer.dev',
    href: 'docs.hexlayer.dev',
  },
  {
    id: 'community',
    icon: Github,
    label: 'Community',
    desc: '14,200 developers in Discord & GitHub Discussions. Office hours every Thursday.',
    meta: 'discord.gg/hexlayer',
    href: 'github.com/hexlayer',
  },
];

const topics = ['Sales inquiry', 'Bug report', 'API access', 'Partnership', 'Security', 'Other'];

export default function App() {
  const [topic, setTopic] = useState('Sales inquiry');
  const [form, setForm] = useState({ name: '', email: '', company: '', message: '' });
  const [submitted, setSubmitted] = useState(false);
  const [copied, setCopied] = useState(false);
  const [time, setTime] = useState(new Date());

  useEffect(() => {
    const t = setInterval(() => setTime(new Date()), 1000);
    return () => clearInterval(t);
  }, []);

  const handleSubmit = (e) => {
    e.preventDefault();
    setSubmitted(true);
  };

  const copyEmail = () => {
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const utc = time.toUTCString().slice(17, 25);

  return (
    <div className="min-h-screen bg-[#0a0c10] text-[#e6e9ef] antialiased selection:bg-[#9aff5e] selection:text-[#0a0c10]">
      <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet" />
      <style dangerouslySetInnerHTML={{ __html: `
        :root { color-scheme: dark; }
        body { margin: 0; }
        .font-mono-x { font-family: 'JetBrains Mono', monospace; }
        .font-sans-x { font-family: 'Inter', sans-serif; }
        .grid-bg {
          background-image:
            linear-gradient(rgba(255,255,255,0.025) 1px, transparent 1px),
            linear-gradient(90deg, rgba(255,255,255,0.025) 1px, transparent 1px);
          background-size: 56px 56px;
        }
        .scanline {
          background: repeating-linear-gradient(0deg, transparent 0px, transparent 2px, rgba(154,255,94,0.012) 2px, rgba(154,255,94,0.012) 4px);
        }
        @keyframes blink { 0%, 49% { opacity: 1; } 50%, 100% { opacity: 0; } }
        .cursor-blink { animation: blink 1.1s steps(1) infinite; }
        @keyframes pulse-dot { 0%, 100% { box-shadow: 0 0 0 0 rgba(154,255,94,0.4); } 50% { box-shadow: 0 0 0 5px rgba(154,255,94,0); } }
        .pulse-dot { animation: pulse-dot 2s ease-in-out infinite; }
        input::placeholder, textarea::placeholder { color: #4a5160; }
        input:focus, textarea:focus { outline: none; }
        .field-line { transition: border-color 0.2s ease, background 0.2s ease; }
        .field-line:focus-within { border-color: #9aff5e; background: rgba(154,255,94,0.025); }
        ::-webkit-scrollbar { width: 10px; }
        ::-webkit-scrollbar-track { background: #0a0c10; }
        ::-webkit-scrollbar-thumb { background: #1e232e; border-radius: 5px; }
      `}} />

      <div className="grid-bg min-h-screen">
        <div className="scanline min-h-screen">

          {/* Top bar */}
          <header className="border-b border-[#1a1f29]">
            <div className="max-w-[1320px] mx-auto px-6 lg:px-10 h-16 flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="w-8 h-8 bg-[#9aff5e] flex items-center justify-center">
                  <Terminal className="w-4.5 h-4.5 text-[#0a0c10]" size={18} strokeWidth={2.5} />
                </div>
                <span className="font-sans-x font-700 font-bold tracking-tight text-[17px]">hexlayer</span>
                <span className="font-mono-x text-[11px] text-[#4a5160] mt-0.5">v3.2.1</span>
              </div>
              <nav className="hidden md:flex items-center gap-8 font-mono-x text-[13px] text-[#8b93a7]">
                <a href="#" className="hover:text-white transition-colors">/docs</a>
                <a href="#" className="hover:text-white transition-colors">/pricing</a>
                <a href="#" className="hover:text-white transition-colors">/changelog</a>
                <a href="#" className="text-[#9aff5e] flex items-center gap-1.5">
                  <span className="w-1.5 h-1.5 rounded-full bg-[#9aff5e] pulse-dot" />
                  /contact
                </a>
              </nav>
              <div className="font-mono-x text-[12px] text-[#4a5160] hidden lg:flex items-center gap-2">
                <Clock size={12} />
                {utc} UTC
              </div>
            </div>
          </header>

          <main className="max-w-[1320px] mx-auto px-6 lg:px-10 pt-16 lg:pt-24 pb-24">

            {/* Hero row */}
            <div className="grid lg:grid-cols-12 gap-12 lg:gap-8 mb-16 lg:mb-24">
              <div className="lg:col-span-7">
                <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.5 }}>
                  <p className="font-mono-x text-[13px] text-[#9aff5e] mb-6 flex items-center gap-2">
                    <span className="text-[#4a5160]">$</span> hexlayer contact --human
                    <span className="cursor-blink inline-block w-2 h-4 bg-[#9aff5e] align-middle" />
                  </p>
                  <h1 className="font-sans-x font-bold tracking-[-0.035em] text-[clamp(2.6rem,6vw,4.5rem)] leading-[1.02]">
                    Talk to the people<br />
                    <span className="text-[#5b6475]">who ship the tool.</span>
                  </h1>
                  <p className="font-sans-x text-[#8b93a7] text-lg leading-relaxed mt-7 max-w-[34rem]">
                    No ticket queues routed offshore. Every message lands with an engineer
                    on the team that builds Hexlayer — usually the one who wrote the code you're asking about.
                  </p>
                </motion.div>
              </div>

              <div className="lg:col-span-5 flex lg:justify-end items-end">
                <motion.div
                  initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.5, delay: 0.1 }}
                  className="border border-[#1e2530] bg-[#0d1015] p-6 w-full max-w-sm"
                >
                  <div className="flex items-center justify-between mb-5">
                    <span className="font-mono-x text-[11px] uppercase tracking-[0.18em] text-[#5b6475]">System status</span>
                    <span className="font-mono-x text-[11px] text-[#9aff5e] flex items-center gap-2">
                      <span className="w-1.5 h-1.5 rounded-full bg-[#9aff5e] pulse-dot" />
                      All operational
                    </span>
                  </div>
                  <div className="space-y-3 font-mono-x text-[13px]">
                    {[
                      ['Build pipeline', '99.99%'],
                      ['Edge runtime', '99.97%'],
                      ['Registry API', '100.0%'],
                      ['Dashboard', '99.95%'],
                    ].map(([name, val]) => (
                      <div key={name} className="flex items-center justify-between border-b border-[#161b24] pb-3 last:border-0 last:pb-0">
                        <span className="text-[#8b93a7]">{name}</span>
                        <span className="text-[#e6e9ef]">{val}</span>
                      </div>
                    ))}
                  </div>
                  <a href="#" className="font-mono-x text-[12px] text-[#5b6475] hover:text-[#9aff5e] transition-colors flex items-center gap-1 mt-5">
                    status.hexlayer.dev <ArrowUpRight size={13} />
                  </a>
                </motion.div>
              </div>
            </div>

            {/* Main grid */}
            <div className="grid lg:grid-cols-12 gap-12 lg:gap-16">

              {/* Channels */}
              <div className="lg:col-span-5 order-2 lg:order-1">
                <p className="font-mono-x text-[11px] uppercase tracking-[0.18em] text-[#5b6475] mb-6">// Pick a channel</p>
                <div className="space-y-px">
                  {channels.map((c, i) => (
                    <motion.a
                      key={c.id}
                      href="#"
                      initial={{ opacity: 0, x: -12 }}
                      animate={{ opacity: 1, x: 0 }}
                      transition={{ duration: 0.4, delay: 0.15 + i * 0.07 }}
                      className="group flex items-start gap-5 border border-[#1a1f29] bg-[#0c0f14] hover:bg-[#10141b] hover:border-[#2a3342] p-5 lg:p-6 transition-all -mt-px first:mt-0"
                    >
                      <div className="w-10 h-10 shrink-0 border border-[#222a37] flex items-center justify-center text-[#9aff5e] group-hover:bg-[#9aff5e] group-hover:text-[#0a0c10] group-hover:border-[#9aff5e] transition-all">
                        <c.icon size={18} strokeWidth={1.75} />
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between">
                          <h3 className="font-sans-x font-semibold text-[15px]">{c.label}</h3>
                          <ArrowUpRight size={16} className="text-[#3a4252] group-hover:text-[#9aff5e] group-hover:translate-x-0.5 group-hover:-translate-y-0.5 transition-all" />
                        </div>
                        <p className="font-sans-x text-[13.5px] text-[#7a8296] leading-relaxed mt-1.5">{c.desc}</p>
                        <p className="font-mono-x text-[12px] text-[#4a5160] mt-3">{c.meta}</p>
                      </div>
                    </motion.a>
                  ))}
                </div>

                {/* Direct email */}
                <div className="mt-8 border border-dashed border-[#222a37] p-5 flex items-center justify-between gap-4">
                  <div>
                    <p className="font-mono-x text-[11px] uppercase tracking-[0.18em] text-[#5b6475] mb-1.5">Prefer plain email?</p>
                    <p className="font-mono-x text-[14px] text-[#e6e9ef]">hello@hexlayer.dev</p>
                  </div>
                  <button
                    onClick={copyEmail}
                    className="font-mono-x text-[12px] flex items-center gap-2 px-3.5 py-2 border border-[#222a37] text-[#8b93a7] hover:text-[#9aff5e] hover:border-[#9aff5e]/40 transition-all"
                  >
                    {copied ? <Check size={13} className="text-[#9aff5e]" /> : <Copy size={13} />}
                    {copied ? 'copied' : 'copy'}
                  </button>
                </div>

                {/* Offices */}
                <div className="mt-8 grid grid-cols-2 gap-6 font-mono-x text-[12px]">
                  <div>
                    <p className="text-[#5b6475] flex items-center gap-1.5 mb-2"><Globe size={12} /> SAN FRANCISCO</p>
                    <p className="text-[#8b93a7] leading-relaxed">580 Howard St, Floor 4<br />CA 94105 · GMT-8</p>
                  </div>
                  <div>
                    <p className="text-[#5b6475] flex items-center gap-1.5 mb-2"><Globe size={12} /> AMSTERDAM</p>
                    <p className="text-[#8b93a7] leading-relaxed">Herengracht 182<br />1016 BR · GMT+1</p>
                  </div>
                </div>
              </div>

              {/* Form */}
              <div className="lg:col-span-7 order-1 lg:order-2">
                <motion.div
                  initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.5, delay: 0.2 }}
                  className="border border-[#1e2530] bg-[#0d1015]"
                >
                  {/* terminal chrome */}
                  <div className="flex items-center justify-between px-5 py-3.5 border-b border-[#1a1f29]">
                    <div className="flex items-center gap-2">
                      <span className="w-3 h-3 rounded-full bg-[#2a3342]" />
                      <span className="w-3 h-3 rounded-full bg-[#2a3342]" />
                      <span className="w-3 h-3 rounded-full bg-[#2a3342]" />
                    </div>
                    <span className="font-mono-x text-[12px] text-[#5b6475]">~/contact/new-message</span>
                    <span className="font-mono-x text-[11px] text-[#3a4252]">utf-8</span>
                  </div>

                  <AnimatePresence mode="wait">
                    {!submitted ? (
                      <motion.form
                        key="form"
                        onSubmit={handleSubmit}
                        exit={{ opacity: 0, y: -10 }}
                        className="p-6 lg:p-9"
                      >
                        {/* Topic chips */}
                        <p className="font-mono-x text-[11px] uppercase tracking-[0.18em] text-[#5b6475] mb-4">01 — What's this about?</p>
                        <div className="flex flex-wrap gap-2 mb-9">
                          {topics.map((t) => (
                            <button
                              key={t}
                              type="button"
                              onClick={() => setTopic(t)}
                              className={`font-mono-x text-[12.5px] px-3.5 py-2 border transition-all ${
                                topic === t
                                  ? 'bg-[#9aff5e] text-[#0a0c10] border-[#9aff5e] font-medium'
                                  : 'border-[#222a37] text-[#8b93a7] hover:border-[#3a4252] hover:text-white'
                              }`}
                            >
                              {topic === t ? <CheckCircle2 size={12} className="inline mr-1.5 -mt-px" /> : <Circle size={12} className="inline mr-1.5 -mt-px opacity-40" />}
                              {t}
                            </button>
                          ))}
                        </div>

                        <p className="font-mono-x text-[11px] uppercase tracking-[0.18em] text-[#5b6475] mb-4">02 — Who are you?</p>
                        <div className="grid sm:grid-cols-2 gap-4 mb-4">
                          <label className="field-line border border-[#222a37] bg-[#0a0d12] block">
                            <span className="font-mono-x text-[11px] text-[#5b6475] block px-4 pt-3">name</span>
                            <input
                              required
                              value={form.name}
                              onChange={(e) => setForm({ ...form, name: e.target.value })}
                              placeholder="Ada Lovelace"
                              className="w-full bg-transparent font-sans-x text-[15px] px-4 pb-3 pt-1 text-white"
                            />
                          </label>
                          <label className="field-line border border-[#222a37] bg-[#0a0d12] block">
                            <span className="font-mono-x text-[11px] text-[#5b6475] block px-4 pt-3">work_email</span>
                            <input
                              required
                              type="email"
                              value={form.email}
                              onChange={(e) => setForm({ ...form, email: e.target.value })}
                              placeholder="ada@analytical.engine"
                              className="w-full bg-transparent font-sans-x text-[15px] px-4 pb-3 pt-1 text-white"
                            />
                          </label>
                        </div>
                        <label className="field-line border border-[#222a37] bg-[#0a0d12] block mb-9">
                          <span className="font-mono-x text-[11px] text-[#5b6475] block px-4 pt-3">company <span className="text-[#3a4252]">(optional)</span></span>
                          <input
                            value={form.company}
                            onChange={(e) => setForm({ ...form, company: e.target.value })}
                            placeholder="Acme Robotics — 40 engineers"
                            className="w-full bg-transparent font-sans-x text-[15px] px-4 pb-3 pt-1 text-white"
                          />
                        </label>

                        <p className="font-mono-x text-[11px] uppercase tracking-[0.18em] text-[#5b6475] mb-4">03 — The details</p>
                        <label className="field-line border border-[#222a37] bg-[#0a0d12] block mb-7">
                          <span className="font-mono-x text-[11px] text-[#5b6475] block px-4 pt-3">message</span>
                          <textarea
                            required
                            rows={5}
                            value={form.message}
                            onChange={(e) => setForm({ ...form, message: e.target.value })}
                            placeholder="We're migrating 200+ services to Hexlayer's build cache and need to understand SSO + audit log support before procurement signs off…"
                            className="w-full bg-transparent font-sans-x text-[15px] px-4 pb-4 pt-1 text-white resize-none leading-relaxed"
                          />
                        </label>

                        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-5">
                          <p className="font-mono-x text-[12px] text-[#4a5160] leading-relaxed">
                            Routed by topic · No marketing follow-ups<br />Avg first reply: <span className="text-[#9aff5e]">2h 14m</span>
                          </p>
                          <button
                            type="submit"
                            className="group font-mono-x text-[14px] font-medium bg-[#9aff5e] text-[#0a0c10] px-7 py-3.5 flex items-center justify-center gap-2 hover:bg-[#b4ff85] transition-colors"
                          >
                            send_message()
                            <ChevronRight size={16} className="group-hover:translate-x-0.5 transition-transform" />
                          </button>
                        </div>
                      </motion.form>
                    ) : (
                      <motion.div
                        key="done"
                        initial={{ opacity: 0, y: 10 }}
                        animate={{ opacity: 1, y: 0 }}
                        className="p-6 lg:p-9 font-mono-x text-[14px] leading-loose"
                      >
                        <p className="text-[#5b6475]">$ hexlayer contact send --topic "{topic.toLowerCase()}"</p>
                        <p className="text-[#8b93a7] mt-2">→ validating payload <span className="text-[#9aff5e]">✓</span></p>
                        <p className="text-[#8b93a7]">→ routing to {topic === 'Sales inquiry' ? 'sales-eng' : 'core-runtime'} on-call <span className="text-[#9aff5e]">✓</span></p>
                        <p className="text-[#8b93a7]">→ confirmation sent to <span className="text-white">{form.email || 'your inbox'}</span> <span className="text-[#9aff5e]">✓</span></p>
                        <div className="border border-[#9aff5e]/25 bg-[#9aff5e]/[0.04] p-5 mt-6">
                          <p className="text-[#9aff5e] font-medium flex items-center gap-2">
                            <CheckCircle2 size={16} /> Message #HX-48217 created
                          </p>
                          <p className="font-sans-x text-[#8b93a7] text-[14px] leading-relaxed mt-2">
                            Thanks, {form.name.split(' ')[0] || 'friend'}. An engineer will reply within a few hours
                            (Mon–Fri, SF & Amsterdam hours). Urgent production issue? Ping us in Discord — it pages on-call.
                          </p>
                        </div>
                        <button
                          onClick={() => { setSubmitted(false); setForm({ name: '', email: '', company: '', message: '' }); }}
                          className="mt-6 text-[12.5px] text-[#5b6475] hover:text-[#9aff5e] transition-colors"
                        >
                          ↺ send another message
                        </button>
                      </motion.div>
                    )}
                  </AnimatePresence>
                </motion.div>

                {/* Trust strip */}
                <div className="mt-6 flex flex-wrap items-center gap-x-8 gap-y-3 font-mono-x text-[12px] text-[#4a5160]">
                  <span>SOC 2 Type II</span>
                  <span className="w-1 h-1 rounded-full bg-[#2a3342]" />
                  <span>GDPR / DPA on request</span>
                  <span className="w-1 h-1 rounded-full bg-[#2a3342]" />
                  <span>Trusted by Vercel, Ramp, Linear & 9,000+ teams</span>
                </div>
              </div>
            </div>
          </main>

          {/* Footer */}
          <footer className="border-t border-[#1a1f29]">
            <div className="max-w-[1320px] mx-auto px-6 lg:px-10 h-16 flex items-center justify-between font-mono-x text-[12px] text-[#4a5160]">
              <span>© 2025 Hexlayer Systems, Inc.</span>
              <div className="flex items-center gap-6">
                <a href="#" className="hover:text-[#9aff5e] transition-colors flex items-center gap-1.5"><Github size={13} /> github</a>
                <a href="#" className="hover:text-[#9aff5e] transition-colors flex items-center gap-1.5"><Mail size={13} /> hello@hexlayer.dev</a>
              </div>
            </div>
          </footer>

        </div>
      </div>
    </div>
  );
}