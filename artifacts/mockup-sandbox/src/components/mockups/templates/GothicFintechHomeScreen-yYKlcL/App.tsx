import { useState } from 'react';
import { motion } from 'framer-motion';
import {
  Compass, ArrowUpRight, ArrowDownLeft, Check, Minus, Crown,
  Landmark, Flame, ScrollText, ChevronRight, Bell, Eye, EyeOff
} from 'lucide-react';

const reveal = {
  hidden: { opacity: 0, y: 36 },
  show: { opacity: 1, y: 0, transition: { duration: 0.9, ease: [0.16, 1, 0.3, 1] } },
};

const Section = ({ children, className = '', delay = 0 }) => (
  <motion.section
    variants={reveal}
    initial="hidden"
    whileInView="show"
    viewport={{ once: true, amount: 0.25 }}
    transition={{ delay }}
    className={className}
  >
    {children}
  </motion.section>
);

const tiers = [
  {
    id: 'crypt',
    name: 'The Crypt',
    glyph: '✛',
    icon: Flame,
    tagline: 'A single chamber, transformed.',
    project: 4800,
    retainer: 1200,
    accent: '#6b6557',
    features: ['One room concept & moodboard', '2 design revisions', 'Sourced furniture list', 'Trade pricing on 40+ vendors'],
  },
  {
    id: 'nave',
    name: 'The Nave',
    glyph: '⚜',
    icon: Landmark,
    tagline: 'The full residence, reimagined in shadow & light.',
    project: 14500,
    retainer: 3400,
    accent: '#9a2433',
    featured: true,
    features: ['Whole-home design direction', 'Unlimited revisions for 90 days', '3D walkthrough renders', 'Contractor & artisan management', 'Trade pricing on 200+ vendors'],
  },
  {
    id: 'cathedral',
    name: 'The Cathedral',
    glyph: '✠',
    icon: Crown,
    tagline: 'Estates, hospitality, the unrepeatable.',
    project: 42000,
    retainer: 8800,
    accent: '#b08d4f',
    features: ['Architectural collaboration', 'Bespoke furniture commissions', 'Dedicated studio team of 4', 'Site visits, unlimited', 'White-glove install & styling'],
  },
];

const matrix = [
  { label: 'Concept boards', vals: ['1 room', 'Whole home', 'Unlimited'] },
  { label: 'Revisions', vals: ['2', '∞ / 90 days', '∞ / 1 year'] },
  { label: '3D renders', vals: [false, true, true] },
  { label: 'Site visits', vals: [false, '4', '∞'] },
  { label: 'Custom millwork', vals: [false, false, true] },
  { label: 'Trade discount', vals: ['12%', '22%', '30%'] },
];

const ledger = [
  { name: 'Maison Vervane — drapery', date: 'Feb 12', amt: -2840, type: 'out' },
  { name: 'Holloway Estate — retainer', date: 'Feb 10', amt: 8800, type: 'in' },
  { name: 'Obsidian Stoneworks', date: 'Feb 08', amt: -6120, type: 'out' },
  { name: 'Ashcroft Penthouse — phase II', date: 'Feb 05', amt: 14500, type: 'in' },
  { name: 'Candlewick Forge — ironwork', date: 'Feb 03', amt: -1975, type: 'out' },
];

const fmt = (n) => n.toLocaleString('en-US');

export default function App() {
  const [mode, setMode] = useState('project');
  const [hidden, setHidden] = useState(false);
  const [activeTier, setActiveTier] = useState('nave');

  return (
    <div className="min-h-screen w-full bg-[#0a0908] text-[#e6e0d2] antialiased" style={{ fontFamily: "'Inter', sans-serif", fontSize: 14 }}>
      <link href="https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,500;0,600;1,400;1,500&family=UnifrakturMaguntia&family=Inter:wght@300;400;500;600&display=swap" rel="stylesheet" />
      <style dangerouslySetInnerHTML={{ __html: `
        ::selection { background: #9a2433; color: #f2ecdd; }
        body { background: #0a0908; }
        .arch { border-radius: 999px 999px 14px 14px; }
        .arch-sm { border-radius: 999px 999px 8px 8px; }
        .grain::before {
          content: ''; position: fixed; inset: 0; pointer-events: none; z-index: 50; opacity: .35;
          background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.06'/%3E%3C/svg%3E");
        }
        .rule { background: linear-gradient(90deg, transparent, rgba(176,141,79,.55), transparent); height: 1px; }
        .gothic-num { font-family: 'Cormorant Garamond', serif; font-feature-settings: 'lnum'; }
        ::-webkit-scrollbar { width: 10px; }
        ::-webkit-scrollbar-track { background: #0a0908; }
        ::-webkit-scrollbar-thumb { background: #2a2620; border-radius: 0; }
        @keyframes flicker { 0%,100%{opacity:1} 48%{opacity:.85} 52%{opacity:.95} 70%{opacity:.8} }
        .flicker { animation: flicker 6s infinite; }
      `}} />

      <div className="grain" />

      <div className="mx-auto w-full max-w-[640px] px-6 pb-32">

        {/* ───────── Header ───────── */}
        <Section className="pt-10 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="arch-sm flex h-10 w-8 items-center justify-center border border-[#3a342a] bg-[#13110e]">
              <Compass size={15} className="text-[#b08d4f]" strokeWidth={1.5} />
            </div>
            <div>
              <div style={{ fontFamily: "'UnifrakturMaguntia', serif" }} className="text-[20px] leading-none text-[#f2ecdd]">Wanderkeep</div>
              <div className="mt-0.5 text-[10px] uppercase tracking-[0.3em] text-[#7d7565]">Studio Treasury</div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <button className="relative flex h-9 w-9 items-center justify-center rounded-full border border-[#2a2620] text-[#a39a86] transition-colors hover:border-[#b08d4f] hover:text-[#b08d4f]">
              <Bell size={15} strokeWidth={1.5} />
              <span className="absolute right-2 top-2 h-1.5 w-1.5 rounded-full bg-[#9a2433]" />
            </button>
            <img src="https://images.unsplash.com/photo-1531746020798-e6953c6e8e04?w=100&h=100&fit=crop" alt="" className="h-9 w-9 rounded-full border border-[#3a342a] object-cover grayscale" />
          </div>
        </Section>

        {/* ───────── Balance / arch card ───────── */}
        <Section className="mt-12">
          <div className="arch relative overflow-hidden border border-[#2a2620] bg-gradient-to-b from-[#161310] via-[#100e0b] to-[#0c0a08] px-8 pb-8 pt-16 text-center">
            <div className="pointer-events-none absolute inset-x-0 top-0 h-40 flicker" style={{ background: 'radial-gradient(ellipse at 50% 0%, rgba(176,141,79,0.16), transparent 70%)' }} />
            <div className="text-[10px] uppercase tracking-[0.4em] text-[#7d7565]">Studio Balance</div>
            <div className="mt-4 flex items-baseline justify-center gap-2">
              <span className="gothic-num text-[24px] text-[#b08d4f]">$</span>
              <span className="gothic-num text-[64px] font-medium leading-none tracking-tight text-[#f2ecdd]">
                {hidden ? '••••••' : '128,460'}
              </span>
            </div>
            <button onClick={() => setHidden(!hidden)} className="mx-auto mt-3 flex items-center gap-1.5 text-[11px] uppercase tracking-[0.2em] text-[#7d7565] transition-colors hover:text-[#b08d4f]">
              {hidden ? <Eye size={12} /> : <EyeOff size={12} />} {hidden ? 'Reveal' : 'Conceal'}
            </button>

            <div className="rule my-6" />

            <div className="grid grid-cols-3 gap-2 text-left">
              {[
                { k: 'Inflow · Feb', v: '+$23,300', c: '#9bb08a' },
                { k: 'Outflow · Feb', v: '−$10,935', c: '#c2554f' },
                { k: 'Reserved', v: '$41,200', c: '#b08d4f' },
              ].map((m) => (
                <div key={m.k} className="rounded-md border border-[#221f1a] bg-[#0d0b09] px-3 py-3">
                  <div className="text-[9px] uppercase tracking-[0.18em] text-[#6e6657]">{m.k}</div>
                  <div className="gothic-num mt-1 text-[18px]" style={{ color: m.c }}>{m.v}</div>
                </div>
              ))}
            </div>
          </div>
        </Section>

        {/* ───────── Massive heading ───────── */}
        <Section className="mt-24 text-center">
          <div className="text-[11px] uppercase tracking-[0.5em] text-[#9a2433]">Choose your expedition</div>
          <h1
            className="mt-2 leading-[0.85] text-[#f2ecdd]"
            style={{ fontFamily: "'Cormorant Garamond', serif", fontSize: 'clamp(96px, 22vw, 132px)', fontWeight: 500, letterSpacing: '-0.03em' }}
          >
            The<br />
            <em className="text-[#b08d4f]" style={{ fontWeight: 400 }}>Tariff</em>
          </h1>
          <p className="mx-auto mt-6 max-w-[420px] text-[14px] leading-relaxed text-[#a39a86]">
            Three paths through the dark. Every commission is a journey we have not taken before — choose how far you wish to go, and what you're willing to spend to get there.
          </p>
        </Section>

        {/* ───────── Billing toggle ───────── */}
        <Section className="mt-12 flex justify-center">
          <div className="inline-flex border border-[#2a2620] bg-[#100e0b] p-1">
            {[
              { id: 'project', label: 'Per Commission' },
              { id: 'retainer', label: 'Monthly Retainer' },
            ].map((m) => (
              <button
                key={m.id}
                onClick={() => setMode(m.id)}
                className={`px-5 py-2.5 text-[11px] uppercase tracking-[0.2em] transition-all duration-300 ${
                  mode === m.id ? 'bg-[#9a2433] text-[#f2ecdd]' : 'text-[#7d7565] hover:text-[#c9c0ad]'
                }`}
              >
                {m.label}
              </button>
            ))}
          </div>
        </Section>

        {/* ───────── Tier cards ───────── */}
        <div className="mt-10 space-y-6">
          {tiers.map((t, i) => {
            const Icon = t.icon;
            const active = activeTier === t.id;
            const price = mode === 'project' ? t.project : t.retainer;
            return (
              <Section key={t.id} delay={i * 0.05}>
                <button
                  onClick={() => setActiveTier(t.id)}
                  className={`group relative block w-full overflow-hidden border text-left transition-all duration-500 ${
                    active ? 'border-[#b08d4f] bg-[#14110d]' : 'border-[#241f19] bg-[#0e0c0a] hover:border-[#3a342a]'
                  }`}
                  style={{ borderRadius: t.featured ? '160px 160px 12px 12px' : '12px' }}
                >
                  {t.featured && (
                    <div className="pointer-events-none absolute inset-x-0 top-0 h-32" style={{ background: `radial-gradient(ellipse at 50% 0%, ${t.accent}26, transparent 70%)` }} />
                  )}
                  <div className={`px-8 pb-8 ${t.featured ? 'pt-14 text-center' : 'pt-8'}`}>
                    {t.featured && (
                      <div className="mb-4 inline-block border border-[#9a2433] px-3 py-1 text-[9px] uppercase tracking-[0.3em] text-[#d7717e]">
                        Most Chosen
                      </div>
                    )}
                    <div className={`flex items-start ${t.featured ? 'flex-col items-center gap-3' : 'justify-between'}`}>
                      <div>
                        <div className="flex items-center gap-2.5" style={{ justifyContent: t.featured ? 'center' : 'flex-start' }}>
                          <Icon size={16} style={{ color: t.accent }} strokeWidth={1.5} />
                          <h2 style={{ fontFamily: "'Cormorant Garamond', serif" }} className="text-[30px] font-medium leading-none text-[#f2ecdd]">{t.name}</h2>
                        </div>
                        <p className="mt-2 text-[13px] italic text-[#8a8170]" style={{ fontFamily: "'Cormorant Garamond', serif", fontSize: 16 }}>{t.tagline}</p>
                      </div>
                      <div className={t.featured ? 'mt-2 text-center' : 'text-right'}>
                        <div className="gothic-num text-[36px] leading-none text-[#f2ecdd]">
                          <span className="text-[20px] text-[#b08d4f]">$</span>{fmt(price)}
                        </div>
                        <div className="mt-1 text-[10px] uppercase tracking-[0.2em] text-[#6e6657]">
                          {mode === 'project' ? 'per commission' : 'per month'}
                        </div>
                      </div>
                    </div>

                    <div className="rule my-6" />

                    <ul className={`space-y-2.5 ${t.featured ? 'mx-auto max-w-[360px] text-left' : ''}`}>
                      {t.features.map((f) => (
                        <li key={f} className="flex items-start gap-3 text-[14px] text-[#bdb4a0]">
                          <span className="mt-[3px] text-[12px]" style={{ color: t.accent }}>{t.glyph}</span>
                          {f}
                        </li>
                      ))}
                    </ul>

                    <div className={`mt-7 flex items-center gap-2 text-[11px] uppercase tracking-[0.25em] transition-colors ${active ? 'text-[#b08d4f]' : 'text-[#6e6657] group-hover:text-[#a39a86]'} ${t.featured ? 'justify-center' : ''}`}>
                      {active ? <><Check size={13} /> Selected path</> : <>Mark this path <ChevronRight size={13} /></>}
                    </div>
                  </div>
                </button>
              </Section>
            );
          })}
        </div>

        {/* ───────── Comparison matrix ───────── */}
        <Section className="mt-24">
          <div className="mb-8 text-center">
            <div className="text-[11px] uppercase tracking-[0.5em] text-[#9a2433]">Side by side</div>
            <h2 style={{ fontFamily: "'Cormorant Garamond', serif", fontSize: 'clamp(72px,18vw,120px)', fontWeight: 500, letterSpacing: '-0.03em' }} className="leading-[0.9] text-[#f2ecdd]">
              Compare
            </h2>
          </div>

          <div className="overflow-hidden rounded-xl border border-[#241f19] bg-[#0e0c0a]">
            <div className="grid grid-cols-[1.4fr_1fr_1fr_1fr] border-b border-[#241f19] bg-[#13100d] px-5 py-4 text-[10px] uppercase tracking-[0.18em] text-[#7d7565]">
              <div>Provision</div>
              <div className="text-center">Crypt</div>
              <div className="text-center text-[#d7717e]">Nave</div>
              <div className="text-center text-[#b08d4f]">Cathedral</div>
            </div>
            {matrix.map((row, i) => (
              <div key={row.label} className={`grid grid-cols-[1.4fr_1fr_1fr_1fr] items-center px-5 py-4 text-[13px] ${i !== matrix.length - 1 ? 'border-b border-[#1a1713]' : ''}`}>
                <div className="text-[#bdb4a0]">{row.label}</div>
                {row.vals.map((v, j) => (
                  <div key={j} className="text-center gothic-num text-[16px] text-[#e6e0d2]">
                    {v === true ? <Check size={15} className="mx-auto text-[#9bb08a]" /> : v === false ? <Minus size={14} className="mx-auto text-[#3a342a]" /> : v}
                  </div>
                ))}
              </div>
            ))}
            <div className="grid grid-cols-[1.4fr_1fr_1fr_1fr] border-t border-[#2a2620] bg-[#13100d] px-5 py-5">
              <div className="text-[10px] uppercase tracking-[0.18em] text-[#7d7565]">{mode === 'project' ? 'Commission' : 'Retainer / mo'}</div>
              {tiers.map((t) => (
                <div key={t.id} className="gothic-num text-center text-[18px] text-[#f2ecdd]">${fmt(mode === 'project' ? t.project : t.retainer)}</div>
              ))}
            </div>
          </div>
        </Section>

        {/* ───────── Ledger ───────── */}
        <Section className="mt-24">
          <div className="mb-6 flex items-end justify-between">
            <div>
              <div className="flex items-center gap-2 text-[11px] uppercase tracking-[0.4em] text-[#9a2433]">
                <ScrollText size={13} /> The Ledger
              </div>
              <h3 style={{ fontFamily: "'Cormorant Garamond', serif" }} className="mt-1 text-[34px] font-medium leading-none text-[#f2ecdd]">Recent passages</h3>
            </div>
            <button className="text-[11px] uppercase tracking-[0.2em] text-[#7d7565] transition-colors hover:text-[#b08d4f]">View all</button>
          </div>

          <div className="divide-y divide-[#1a1713] rounded-xl border border-[#241f19] bg-[#0e0c0a]">
            {ledger.map((l) => (
              <div key={l.name} className="group flex items-center justify-between px-5 py-4 transition-colors hover:bg-[#13100d]">
                <div className="flex items-center gap-4">
                  <div className={`arch-sm flex h-10 w-8 items-center justify-center border ${l.type === 'in' ? 'border-[#3b4a33] text-[#9bb08a]' : 'border-[#3a2a26] text-[#c2554f]'}`}>
                    {l.type === 'in' ? <ArrowDownLeft size={14} /> : <ArrowUpRight size={14} />}
                  </div>
                  <div>
                    <div className="text-[14px] text-[#e6e0d2]">{l.name}</div>
                    <div className="mt-0.5 text-[11px] text-[#6e6657]">{l.date} · 2025</div>
                  </div>
                </div>
                <div className={`gothic-num text-[18px] ${l.type === 'in' ? 'text-[#9bb08a]' : 'text-[#c9c0ad]'}`}>
                  {l.type === 'in' ? '+' : '−'}${fmt(Math.abs(l.amt))}
                </div>
              </div>
            ))}
          </div>
        </Section>

        {/* ───────── CTA ───────── */}
        <Section className="mt-24 text-center">
          <div className="arch border border-[#2a2620] bg-gradient-to-b from-[#171210] to-[#0c0a08] px-8 pb-10 pt-20">
            <div style={{ fontFamily: "'UnifrakturMaguntia', serif" }} className="text-[28px] text-[#b08d4f]">W</div>
            <h3 style={{ fontFamily: "'Cormorant Garamond', serif" }} className="mt-3 text-[40px] font-medium leading-[1.05] text-[#f2ecdd]">
              Begin the<br /><em>commission</em>
            </h3>
            <p className="mx-auto mt-4 max-w-[360px] text-[14px] leading-relaxed text-[#a39a86]">
              Funds are held in escrow until each phase of the journey is complete. No surprises — only the ones we design.
            </p>
            <button className="group mt-8 inline-flex items-center gap-3 bg-[#9a2433] px-8 py-4 text-[12px] uppercase tracking-[0.3em] text-[#f2ecdd] transition-all duration-300 hover:bg-[#b12a3b] hover:tracking-[0.4em]">
              Reserve {tiers.find((t) => t.id === activeTier)?.name}
              <ArrowUpRight size={15} className="transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
            </button>
            <div className="mt-6 text-[11px] uppercase tracking-[0.2em] text-[#6e6657]">
              {mode === 'project' ? `$${fmt(tiers.find((t) => t.id === activeTier)?.project)} · escrowed` : `$${fmt(tiers.find((t) => t.id === activeTier)?.retainer)} / month · cancel anytime`}
            </div>
          </div>
          <div className="mt-10 text-[10px] uppercase tracking-[0.35em] text-[#4d473c]">
            Wanderkeep Studio Treasury · Member FDIC · Est. MMXIX
          </div>
        </Section>
      </div>
    </div>
  );
}