import { useState, useEffect, useRef } from 'react';
import { motion } from 'framer-motion';
import { ArrowRight, Plus } from 'lucide-react';

const PHRASE = 'Time, wrought in darkness.';

const NAV = [
  { id: 'atelier', num: 'I', label: 'The Atelier' },
  { id: 'calibre', num: 'II', label: 'Calibre 1847' },
  { id: 'provenance', num: 'III', label: 'Provenance' },
  { id: 'viewing', num: 'IV', label: 'Private Viewing' },
];

const fadeUp = {
  hidden: { opacity: 0, y: 24 },
  show: { opacity: 1, y: 0, transition: { duration: 0.7, ease: [0.22, 1, 0.36, 1] } },
};

function Arch({ src, w, h, alt, className = '' }) {
  // gothic lancet arch clip
  const spring = h * 0.34;
  const path = `M0,${h} L0,${spring} C0,${spring * 0.42} ${w * 0.16},${spring * 0.14} ${w / 2},0 C${w * 0.84},${spring * 0.14} ${w},${spring * 0.42} ${w},${spring} L${w},${h} Z`;
  return (
    <div className={`relative ${className}`} style={{ width: w, height: h }}>
      <div
        className="absolute inset-0"
        style={{ clipPath: `path("${path}")`, background: '#1a1713' }}
      >
        <img
          src={src}
          alt={alt}
          className="w-full h-full object-cover grayscale-[35%] contrast-[1.12] brightness-[0.82] transition-all duration-700 hover:grayscale-0 hover:brightness-[0.95]"
        />
      </div>
      {/* arch outline */}
      <svg className="absolute -inset-[10px] pointer-events-none" width={w + 20} height={h + 20}>
        <path
          d={`M10,${h + 10} L10,${spring + 10} C10,${spring * 0.42 + 10} ${w * 0.16 + 10},${spring * 0.14 + 10} ${w / 2 + 10},10 C${w * 0.84 + 10},${spring * 0.14 + 10} ${w + 10},${spring * 0.42 + 10} ${w + 10},${spring + 10} L${w + 10},${h + 10}`}
          fill="none"
          stroke="#a3884f"
          strokeWidth="1"
          opacity="0.55"
        />
      </svg>
    </div>
  );
}

function SectionRule({ children }) {
  return (
    <div className="flex items-center gap-4 mb-8">
      <span className="h-px flex-1 bg-[#2a2620]" />
      <span className="font-mono text-[10px] tracking-[0.35em] text-[#a3884f] uppercase">{children}</span>
      <span className="text-[#a3884f] text-[9px]">◆</span>
      <span className="h-px flex-1 bg-[#2a2620]" />
    </div>
  );
}

export default function App() {
  const [typed, setTyped] = useState('');
  const [active, setActive] = useState('atelier');
  const refs = {
    atelier: useRef(null),
    calibre: useRef(null),
    provenance: useRef(null),
    viewing: useRef(null),
  };

  useEffect(() => {
    let i = 0;
    const start = setTimeout(() => {
      const t = setInterval(() => {
        i++;
        setTyped(PHRASE.slice(0, i));
        if (i >= PHRASE.length) clearInterval(t);
      }, 65);
    }, 600);
    return () => clearTimeout(start);
  }, []);

  const goTo = (id) => {
    setActive(id);
    refs[id].current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };

  return (
    <div className="min-h-screen bg-[#060504] flex items-start justify-center py-10 px-4 selection:bg-[#a3884f] selection:text-[#0b0a08]">
      <link
        href="https://fonts.googleapis.com/css2?family=Bodoni+Moda:ital,opsz,wght@0,6..96,400;0,6..96,500;0,6..96,600;1,6..96,400&family=IBM+Plex+Mono:ital,wght@0,300;0,400;0,500;1,300&display=swap"
        rel="stylesheet"
      />
      <style
        dangerouslySetInnerHTML={{
          __html: `
        .serif { font-family: 'Bodoni Moda', serif; }
        .mono { font-family: 'IBM Plex Mono', monospace; }
        @keyframes blink { 0%, 49% { opacity: 1; } 50%, 100% { opacity: 0; } }
        .caret { display:inline-block; width: 3px; height: 0.85em; background: #a3884f; margin-left: 6px; animation: blink 1s step-end infinite; transform: translateY(0.08em); }
        .email-scroll::-webkit-scrollbar { width: 5px; }
        .email-scroll::-webkit-scrollbar-track { background: #0d0c0a; }
        .email-scroll::-webkit-scrollbar-thumb { background: #2e2920; }
        .email-scroll::-webkit-scrollbar-thumb:hover { background: #a3884f; }
        .grain::after {
          content:''; position:absolute; inset:0; pointer-events:none; opacity:0.05; mix-blend-mode: overlay;
          background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='3'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
        }
        .dropcap::first-letter {
          font-family: 'Bodoni Moda', serif;
          float: left; font-size: 64px; line-height: 0.78;
          padding: 8px 12px 0 0; color: #a3884f;
        }
        .nav-item { transition: all 0.4s cubic-bezier(0.22,1,0.36,1); }
        .nav-item:hover { letter-spacing: 0.28em; color: #e6ddc9; }
        .cta-btn { transition: all 0.45s cubic-bezier(0.22,1,0.36,1); }
        .cta-btn:hover { background: #a3884f; color: #0b0a08; }
        .cta-btn:hover .cta-arrow { transform: translateX(6px); }
        .cta-arrow { transition: transform 0.45s cubic-bezier(0.22,1,0.36,1); }
        .spec-row { transition: background 0.3s; }
        .spec-row:hover { background: #161310; }
      `,
        }}
      />

      <div className="w-full max-w-[1000px]">
        {/* preheader */}
        <div className="mono text-[10px] tracking-[0.2em] text-[#5b5345] flex justify-between px-2 pb-3 uppercase">
          <span>Mortlake &amp; Sons — Dispatch № 047</span>
          <span className="hover:text-[#a3884f] cursor-pointer transition-colors">View in browser †</span>
        </div>

        {/* email body */}
        <div className="relative grain bg-[#0b0a08] border border-[#26221c] flex" style={{ height: '82vh', minHeight: 640 }}>
          {/* ============ SIDEBAR ============ */}
          <aside className="w-[230px] shrink-0 border-r border-[#26221c] flex flex-col justify-between py-10 px-7 bg-[#0d0b09]">
            <div>
              {/* monogram */}
              <div className="mb-12">
                <div className="w-14 h-[72px] border border-[#a3884f] flex items-center justify-center mb-5"
                  style={{ borderRadius: '50% 50% 0 0 / 62% 62% 0 0' }}>
                  <span className="serif text-[28px] text-[#e6ddc9] leading-none mt-1">M</span>
                </div>
                <h1 className="serif text-[19px] text-[#e6ddc9] leading-tight tracking-wide">
                  Mortlake<br />&amp; Sons
                </h1>
                <p className="mono text-[9px] tracking-[0.3em] text-[#7a6f5c] mt-2 uppercase">Horologists · est. 1847</p>
              </div>

              {/* nav */}
              <nav className="space-y-5">
                {NAV.map((n) => (
                  <button
                    key={n.id}
                    onClick={() => goTo(n.id)}
                    className={`nav-item mono w-full text-left text-[10px] uppercase tracking-[0.22em] flex items-baseline gap-3 ${
                      active === n.id ? 'text-[#e6ddc9]' : 'text-[#6b6151]'
                    }`}
                  >
                    <span className={`serif text-[13px] w-5 ${active === n.id ? 'text-[#a3884f]' : 'text-[#46402f]'}`}>{n.num}</span>
                    <span className="relative">
                      {n.label}
                      {active === n.id && (
                        <motion.span layoutId="navline" className="absolute -bottom-1.5 left-0 right-0 h-px bg-[#a3884f]" />
                      )}
                    </span>
                  </button>
                ))}
              </nav>
            </div>

            <div className="space-y-6">
              <div className="mono text-[9px] leading-relaxed text-[#5b5345] tracking-wider">
                FORGED BY HAND.<br />
                FINISHED BY CANDLELIGHT.<br />
                NUMBERED, NEVER MASS-MADE.
              </div>
              <div className="flex items-center gap-2 text-[#46402f]">
                <span className="h-px w-6 bg-[#2a2620]" />
                <span className="text-[10px]">†</span>
                <span className="h-px w-6 bg-[#2a2620]" />
              </div>
            </div>
          </aside>

          {/* ============ CONTENT ============ */}
          <main className="email-scroll flex-1 overflow-y-auto">
            {/* HERO */}
            <section ref={refs.atelier} className="px-12 pt-14 pb-16 border-b border-[#26221c]">
              <div className="flex gap-10 items-start">
                <div className="flex-1 pt-4">
                  <p className="mono text-[10px] tracking-[0.35em] text-[#a3884f] uppercase mb-7">
                    An invitation from the bench of E. Mortlake
                  </p>
                  <h2 className="serif text-[#e6ddc9] font-medium leading-[1.02]" style={{ fontSize: 'clamp(40px, 4.6vw, 58px)' }}>
                    {typed}
                    <span className="caret" />
                  </h2>
                  <p className="mono text-[12px] leading-[1.9] text-[#9a8f7c] mt-8 max-w-[400px] font-light">
                    For one hundred and seventy-eight years our movements have been
                    assembled in a workshop with no windows facing the street. Not
                    secrecy — concentration. This dispatch concerns the fruit of that
                    silence: the new Calibre 1847 Vesper.
                  </p>
                  <div className="flex items-center gap-6 mt-10">
                    <button className="cta-btn mono text-[10px] tracking-[0.25em] uppercase border border-[#a3884f] text-[#a3884f] px-6 py-3.5 flex items-center gap-3">
                      Read the dispatch <ArrowRight size={13} className="cta-arrow" />
                    </button>
                    <span className="mono text-[10px] text-[#5b5345] tracking-widest">№ 047 / OCT MMXXV</span>
                  </div>
                </div>
                <motion.div initial="hidden" animate="show" variants={fadeUp}>
                  <Arch
                    src="https://images.unsplash.com/photo-1547996160-81dfa63595aa?w=600&h=900&fit=crop"
                    w={250}
                    h={360}
                    alt="Calibre 1847 Vesper"
                  />
                  <p className="mono text-[9px] tracking-[0.25em] text-[#5b5345] uppercase text-center mt-4">
                    Fig. I — The Vesper, tantalum case
                  </p>
                </motion.div>
              </div>
            </section>

            {/* THE ATELIER */}
            <motion.section
              initial="hidden"
              whileInView="show"
              viewport={{ once: true, margin: '-80px' }}
              variants={fadeUp}
              className="px-12 py-14 border-b border-[#26221c]"
            >
              <SectionRule>I · The Atelier</SectionRule>
              <div className="grid grid-cols-[1fr_220px] gap-10 items-center">
                <div>
                  <h3 className="serif text-[30px] text-[#e6ddc9] leading-tight mb-6">
                    Eleven hands. One bench.<br />No clock on the wall.
                  </h3>
                  <p className="dropcap mono text-[12px] leading-[2] text-[#9a8f7c] font-light">
                    Within the stone arches of our Clerkenwell workshop, eleven
                    finishers share a single rule: a component leaves the bench when
                    it is correct, not when it is due. Bridges are bevelled by hand at
                    45 degrees, inspected at thirty-fold magnification, then bevelled
                    again. Screws are flame-blued one at a time over a brass pan —
                    the colour judged by eye, the way it was judged in 1847.
                  </p>
                </div>
                <Arch
                  src="https://images.unsplash.com/photo-1622434641406-a158123450f9?w=500&h=700&fit=crop"
                  w={210}
                  h={300}
                  alt="Watchmaker's bench"
                />
              </div>
            </motion.section>

            {/* CALIBRE 1847 */}
            <motion.section
              ref={refs.calibre}
              initial="hidden"
              whileInView="show"
              viewport={{ once: true, margin: '-80px' }}
              variants={fadeUp}
              className="px-12 py-14 border-b border-[#26221c] bg-[#0d0b09]"
            >
              <SectionRule>II · Calibre 1847 Vesper</SectionRule>
              <div className="flex gap-12">
                <Arch
                  src="https://images.unsplash.com/photo-1509048191080-d2984bad6ae5?w=600&h=900&fit=crop"
                  w={280}
                  h={400}
                  alt="Calibre 1847 movement"
                />
                <div className="flex-1">
                  <h3 className="serif text-[34px] text-[#e6ddc9] leading-[1.05] mb-3">
                    The Vesper
                  </h3>
                  <p className="serif italic text-[16px] text-[#a3884f] mb-8">
                    Forty-one pieces. Never to be repeated.
                  </p>

                  <div className="border-t border-[#2a2620]">
                    {[
                      ['Movement', 'Cal. 1847, hand-wound'],
                      ['Power reserve', '72 hours, twin barrel'],
                      ['Frequency', '21,600 vph'],
                      ['Jewels', '31, set in gold chatons'],
                      ['Case', 'Tantalum, 39 mm, lancet lugs'],
                      ['Dial', 'Black grand feu enamel'],
                      ['Hands', 'Steel, blued by open flame'],
                      ['Edition', '41 pieces, engraved by hand'],
                    ].map(([k, v]) => (
                      <div key={k} className="spec-row mono text-[11px] flex justify-between items-baseline py-[9px] border-b border-[#2a2620] px-2">
                        <span className="text-[#6b6151] uppercase tracking-[0.18em] text-[9px]">{k}</span>
                        <span className="text-[#cfc5af]">{v}</span>
                      </div>
                    ))}
                  </div>

                  <div className="flex items-center justify-between mt-8">
                    <div>
                      <p className="mono text-[9px] uppercase tracking-[0.25em] text-[#6b6151] mb-1">By application</p>
                      <p className="serif text-[24px] text-[#e6ddc9]">£68,400</p>
                    </div>
                    <button className="cta-btn mono text-[10px] tracking-[0.25em] uppercase border border-[#a3884f] text-[#a3884f] px-6 py-3.5 flex items-center gap-3">
                      Request the dossier <ArrowRight size={13} className="cta-arrow" />
                    </button>
                  </div>
                </div>
              </div>
            </motion.section>

            {/* PROVENANCE */}
            <motion.section
              ref={refs.provenance}
              initial="hidden"
              whileInView="show"
              viewport={{ once: true, margin: '-80px' }}
              variants={fadeUp}
              className="px-12 py-14 border-b border-[#26221c]"
            >
              <SectionRule>III · Provenance</SectionRule>
              <div className="grid grid-cols-3 gap-px bg-[#26221c] border border-[#26221c]">
                {[
                  {
                    n: 'MDCCCXLVII',
                    t: 'The first ledger',
                    d: 'Edmund Mortlake records movement № 1, sold to the Bishop of Ely. The ledger is still written by hand today.',
                  },
                  {
                    n: 'MCMXII',
                    t: 'The deck watches',
                    d: 'Forty-one marine chronometers, rated to half a second per day, carried aboard polar expeditions. Three returned.',
                  },
                  {
                    n: 'MMXXV',
                    t: 'The Vesper',
                    d: 'The eleventh calibre in the house archive, and the first cased in tantalum — a metal that resists nearly everything, including haste.',
                  },
                ].map((c) => (
                  <div key={c.n} className="bg-[#0b0a08] p-7 group hover:bg-[#0f0d0a] transition-colors duration-500">
                    <p className="serif text-[15px] text-[#a3884f] mb-5 tracking-wide">{c.n}</p>
                    <h4 className="serif text-[19px] text-[#e6ddc9] mb-3">{c.t}</h4>
                    <p className="mono text-[10.5px] leading-[1.85] text-[#8a8071] font-light">{c.d}</p>
                    <Plus size={12} className="text-[#46402f] mt-5 group-hover:text-[#a3884f] group-hover:rotate-90 transition-all duration-500" />
                  </div>
                ))}
              </div>
            </motion.section>

            {/* PRIVATE VIEWING */}
            <motion.section
              ref={refs.viewing}
              initial="hidden"
              whileInView="show"
              viewport={{ once: true, margin: '-80px' }}
              variants={fadeUp}
              className="px-12 py-16"
            >
              <div className="relative border border-[#a3884f]/40 px-12 py-14 text-center overflow-hidden">
                <div
                  className="absolute inset-0 opacity-[0.14]"
                  style={{
                    backgroundImage:
                      'url(https://images.unsplash.com/photo-1533139502658-0198f920d8e8?w=1200&h=600&fit=crop)',
                    backgroundSize: 'cover',
                    backgroundPosition: 'center',
                    filter: 'grayscale(100%)',
                  }}
                />
                <div className="relative">
                  <p className="mono text-[10px] tracking-[0.35em] text-[#a3884f] uppercase mb-6">IV · Private Viewing — Candlelit</p>
                  <h3 className="serif text-[36px] text-[#e6ddc9] leading-tight mb-4">
                    The crypt opens for three nights only.
                  </h3>
                  <p className="mono text-[11px] text-[#9a8f7c] leading-[1.9] max-w-[460px] mx-auto mb-9 font-light">
                    7 — 9 November · The Charterhouse Crypt, Clerkenwell, London EC1.
                    Twelve guests per evening. The Vesper presented under loupe by the
                    finisher who built it.
                  </p>
                  <button className="cta-btn mono text-[10px] tracking-[0.28em] uppercase bg-[#a3884f] text-[#0b0a08] px-9 py-4 inline-flex items-center gap-3 hover:!bg-[#e6ddc9]">
                    Request a seat <ArrowRight size={13} className="cta-arrow" />
                  </button>
                  <p className="mono text-[9px] tracking-[0.25em] text-[#5b5345] uppercase mt-6">Replies close 31 October at midnight</p>
                </div>
              </div>
            </motion.section>

            {/* FOOTER */}
            <footer className="px-12 pb-12">
              <div className="border-t border-[#26221c] pt-8 flex justify-between items-start">
                <div className="mono text-[9px] leading-[1.9] text-[#5b5345] tracking-wider uppercase">
                  Mortlake &amp; Sons, Horologists<br />
                  14 Charterhouse Square, London EC1M 6AX<br />
                  By appointment only
                </div>
                <div className="mono text-[9px] leading-[1.9] text-[#5b5345] tracking-wider uppercase text-right">
                  <span className="hover:text-[#a3884f] cursor-pointer transition-colors">Manage correspondence</span><br />
                  <span className="hover:text-[#a3884f] cursor-pointer transition-colors">Unsubscribe — we shall not protest</span><br />
                  © MMXXV · Dispatch № 047
                </div>
              </div>
              <div className="flex justify-center mt-8 text-[#46402f] text-[11px] tracking-[1em]">† ◆ †</div>
            </footer>
          </main>
        </div>
      </div>
    </div>
  );
}