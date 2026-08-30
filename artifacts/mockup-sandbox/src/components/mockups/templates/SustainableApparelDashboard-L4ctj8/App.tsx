import { useState } from 'react';
import {
  Leaf, Droplets, Recycle, Wind, TrendingUp, TrendingDown, Search, Bell,
  LayoutGrid, Shirt, Factory, Globe2, BarChart3, Settings, ChevronDown,
  ArrowUpRight, MapPin, Sparkles, Package, RefreshCcw
} from 'lucide-react';
import {
  AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer,
  PieChart, Pie, Cell, BarChart, Bar, CartesianGrid
} from 'recharts';
import { motion } from 'framer-motion';

// ---------- DATA ----------
const revenueImpact = [
  { m: 'Mar', revenue: 182, co2: 38 },
  { m: 'Apr', revenue: 214, co2: 44 },
  { m: 'May', revenue: 198, co2: 47 },
  { m: 'Jun', revenue: 251, co2: 56 },
  { m: 'Jul', revenue: 243, co2: 61 },
  { m: 'Aug', revenue: 289, co2: 67 },
  { m: 'Sep', revenue: 312, co2: 74 },
  { m: 'Oct', revenue: 358, co2: 82 },
];

const materials = [
  { name: 'Organic cotton', value: 42, color: '#3F5C46' },
  { name: 'Recycled wool', value: 23, color: '#C2603D' },
  { name: 'TENCEL™ lyocell', value: 18, color: '#D9A441' },
  { name: 'Hemp blend', value: 11, color: '#8A9B7C' },
  { name: 'Deadstock', value: 6, color: '#B9AE9A' },
];

const recommerce = [
  { m: 'May', resold: 120, repaired: 64 },
  { m: 'Jun', resold: 168, repaired: 82 },
  { m: 'Jul', resold: 154, repaired: 91 },
  { m: 'Aug', resold: 210, repaired: 103 },
  { m: 'Sep', resold: 248, repaired: 118 },
  { m: 'Oct', resold: 297, repaired: 142 },
];

const products = [
  { name: 'Alta Raw-Hem Trouser', cat: 'Organic cotton twill', units: 1842, score: 94, trend: 12.4, img: 'https://images.unsplash.com/photo-1594633312681-425c7b97ccd1?w=120&h=120&fit=crop' },
  { name: 'Mira Boxy Overshirt', cat: 'Recycled wool felt', units: 1530, score: 91, trend: 8.1, img: 'https://images.unsplash.com/photo-1551028719-00167b16eac5?w=120&h=120&fit=crop' },
  { name: 'Field Crew Tee', cat: 'Hemp / cotton jersey', units: 1417, units2: true, score: 88, trend: -2.3, img: 'https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?w=120&h=120&fit=crop' },
  { name: 'Sora Wrap Dress', cat: 'TENCEL™ lyocell', units: 1186, score: 96, trend: 19.7, img: 'https://images.unsplash.com/photo-1595777457583-95e059d581b8?w=120&h=120&fit=crop' },
  { name: 'Loam Utility Jacket', cat: 'Deadstock canvas', units: 904, score: 85, trend: 4.6, img: 'https://images.unsplash.com/photo-1591047139829-d91aecb6caea?w=120&h=120&fit=crop' },
];

const suppliers = [
  { name: 'Khadi Collective', loc: 'Gujarat, India', tier: 'Tier 1 · Cut & Sew', audit: 'A', share: 34, status: 'Audited Sept ’25' },
  { name: 'Lanificio Verde', loc: 'Prato, Italy', tier: 'Tier 2 · Wool recycling', audit: 'A', share: 22, status: 'Audited Jul ’25' },
  { name: 'Aral Fibers Co-op', loc: 'Izmir, Türkiye', tier: 'Tier 3 · Cotton ginning', audit: 'B+', share: 19, status: 'Re-audit due Dec' },
  { name: 'Mekong Dyeworks', loc: 'Hội An, Vietnam', tier: 'Tier 2 · Natural dye', audit: 'A-', share: 14, status: 'Audited Aug ’25' },
];

// ---------- SMALL PIECES ----------
const navItems = [
  { icon: LayoutGrid, label: 'Overview' },
  { icon: Shirt, label: 'Collections' },
  { icon: Leaf, label: 'Impact ledger' },
  { icon: Factory, label: 'Supply chain' },
  { icon: RefreshCcw, label: 'Recommerce' },
  { icon: BarChart3, label: 'Reports' },
];

function Stat({ icon: Icon, label, value, unit, delta, up, sub, accent }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 14 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.45 }}
      className="group relative bg-[#FBF8F1] border border-[#E2DBC9] rounded-[4px] p-5 hover:border-[#1C2B22]/40 transition-colors duration-300"
    >
      <div className="flex items-start justify-between">
        <div className={`w-9 h-9 rounded-full flex items-center justify-center ${accent}`}>
          <Icon size={16} strokeWidth={1.75} />
        </div>
        <span className={`flex items-center gap-1 text-[11px] font-medium tracking-wide px-2 py-[3px] rounded-full ${up ? 'bg-[#3F5C46]/10 text-[#3F5C46]' : 'bg-[#C2603D]/10 text-[#C2603D]'}`}>
          {up ? <TrendingUp size={11} /> : <TrendingDown size={11} />}{delta}
        </span>
      </div>
      <p className="mt-5 text-[11px] uppercase tracking-[0.14em] text-[#6E6A5C]">{label}</p>
      <p className="mt-1 font-fraunces text-[32px] leading-none text-[#1C2B22]">
        {value}<span className="text-[16px] text-[#6E6A5C] ml-1">{unit}</span>
      </p>
      <p className="mt-2 text-[12px] text-[#8A8472]">{sub}</p>
    </motion.div>
  );
}

const ChartTip = ({ active, payload, label }) => {
  if (!active || !payload?.length) return null;
  return (
    <div className="bg-[#1C2B22] text-[#F4F1EA] rounded-[4px] px-3 py-2 text-[12px] shadow-lg">
      <p className="font-medium mb-1">{label}</p>
      {payload.map((p) => (
        <p key={p.dataKey} className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-full" style={{ background: p.color || p.fill }} />
          {p.name}: <span className="font-semibold">{p.value}{p.dataKey === 'revenue' ? 'k' : p.dataKey === 'co2' ? 't' : ''}</span>
        </p>
      ))}
    </div>
  );
};

// ---------- APP ----------
export default function App() {
  const [activeNav, setActiveNav] = useState('Overview');
  const [range, setRange] = useState('8M');

  return (
    <div className="min-h-screen bg-[#F4F1EA] text-[#1C2B22] font-inter antialiased">
      <link rel="preconnect" href="https://fonts.googleapis.com" />
      <link href="https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,300;9..144,400;9..144,500;9..144,600&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet" />
      <style dangerouslySetInnerHTML={{ __html: `
        .font-fraunces { font-family: 'Fraunces', serif; }
        .font-inter { font-family: 'Inter', sans-serif; }
        .grain::before {
          content: ''; position: fixed; inset: 0; pointer-events: none; opacity: 0.35; z-index: 50;
          background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.05'/%3E%3C/svg%3E");
        }
        ::-webkit-scrollbar { width: 10px; height: 10px; }
        ::-webkit-scrollbar-track { background: #EDE8DB; }
        ::-webkit-scrollbar-thumb { background: #C9C0AA; border-radius: 8px; border: 2px solid #EDE8DB; }
        .ring-track { stroke: #E2DBC9; }
        @keyframes drawRing { from { stroke-dashoffset: 264; } }
        .ring-fill { animation: drawRing 1.4s cubic-bezier(.6,.05,.2,1) forwards; }
      `}} />
      <div className="grain" />

      <div className="flex">
        {/* ====== SIDEBAR ====== */}
        <aside className="hidden lg:flex flex-col w-[232px] shrink-0 h-screen sticky top-0 bg-[#1C2B22] text-[#E9E4D5]">
          <div className="px-6 pt-7 pb-8">
            <div className="flex items-center gap-2.5">
              <div className="w-8 h-8 rounded-full bg-[#C2603D] flex items-center justify-center">
                <Leaf size={15} className="text-[#1C2B22]" strokeWidth={2.2} />
              </div>
              <div>
                <p className="font-fraunces text-[19px] leading-none tracking-tight">Tellure</p>
                <p className="text-[10px] tracking-[0.22em] uppercase text-[#8A9B7C] mt-0.5">Studio OS</p>
              </div>
            </div>
          </div>

          <nav className="px-3 space-y-0.5">
            {navItems.map(({ icon: Icon, label }) => {
              const active = activeNav === label;
              return (
                <button
                  key={label}
                  onClick={() => setActiveNav(label)}
                  className={`w-full flex items-center gap-3 px-3 py-[9px] rounded-[4px] text-[13px] transition-all duration-200
                    ${active ? 'bg-[#F4F1EA] text-[#1C2B22] font-semibold' : 'text-[#B9C2AE] hover:bg-white/5 hover:text-[#F4F1EA]'}`}
                >
                  <Icon size={16} strokeWidth={active ? 2.2 : 1.7} />
                  {label}
                  {label === 'Recommerce' && !active && (
                    <span className="ml-auto text-[10px] bg-[#C2603D] text-[#1C2B22] font-bold px-1.5 py-px rounded-full">3</span>
                  )}
                </button>
              );
            })}
          </nav>

          <div className="mt-auto px-4 pb-5">
            <div className="rounded-[6px] bg-[#27392E] p-4 mb-4 border border-white/5">
              <div className="flex items-center gap-2 text-[#D9A441]">
                <Sparkles size={14} />
                <p className="text-[11px] font-semibold tracking-wide uppercase">B-Corp recert</p>
              </div>
              <p className="text-[12px] text-[#B9C2AE] mt-2 leading-relaxed">Audit window opens in 24 days. 7 of 9 documents uploaded.</p>
              <div className="mt-3 h-1.5 rounded-full bg-white/10 overflow-hidden">
                <div className="h-full w-[78%] bg-[#D9A441] rounded-full" />
              </div>
            </div>
            <button className="w-full flex items-center gap-3 px-3 py-2 rounded-[4px] text-[13px] text-[#B9C2AE] hover:bg-white/5">
              <Settings size={16} strokeWidth={1.7} /> Settings
            </button>
          </div>
        </aside>

        {/* ====== MAIN ====== */}
        <main className="flex-1 min-w-0">
          {/* Header */}
          <header className="sticky top-0 z-40 bg-[#F4F1EA]/85 backdrop-blur-md border-b border-[#E2DBC9]">
            <div className="px-6 lg:px-9 py-4 flex items-center gap-4">
              <div>
                <p className="text-[11px] uppercase tracking-[0.18em] text-[#8A8472]">Wednesday, 29 Oct · FW25 drop live</p>
                <h1 className="font-fraunces text-[22px] leading-tight">Good morning, June — the soil report looks great.</h1>
              </div>
              <div className="ml-auto flex items-center gap-3">
                <div className="hidden md:flex items-center gap-2 bg-[#FBF8F1] border border-[#E2DBC9] rounded-full px-3.5 py-2 w-[230px] focus-within:border-[#1C2B22]/40 transition-colors">
                  <Search size={14} className="text-[#8A8472]" />
                  <input placeholder="Search SKUs, mills, batches…" className="bg-transparent text-[13px] outline-none placeholder:text-[#A39B86] w-full" />
                </div>
                <button className="relative w-9 h-9 rounded-full bg-[#FBF8F1] border border-[#E2DBC9] flex items-center justify-center hover:border-[#1C2B22]/40 transition-colors">
                  <Bell size={15} />
                  <span className="absolute top-1.5 right-2 w-1.5 h-1.5 bg-[#C2603D] rounded-full" />
                </button>
                <button className="flex items-center gap-2 pl-1 pr-2.5 py-1 rounded-full bg-[#FBF8F1] border border-[#E2DBC9] hover:border-[#1C2B22]/40 transition-colors">
                  <img src="https://images.unsplash.com/photo-1494790108377-be9c29b29330?w=80&h=80&fit=crop" alt="" className="w-7 h-7 rounded-full object-cover" />
                  <span className="text-[13px] font-medium hidden sm:block">June Okafor</span>
                  <ChevronDown size={14} className="text-[#8A8472]" />
                </button>
              </div>
            </div>
          </header>

          <div className="px-6 lg:px-9 py-7 space-y-7 max-w-[1480px]">
            {/* KPI ROW */}
            <section className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
              <Stat icon={Shirt} label="Garments sold · MTD" value="6,879" unit="" delta="+11.2%" up
                sub="Avg. wears-per-garment up to 41" accent="bg-[#3F5C46]/10 text-[#3F5C46]" />
              <Stat icon={Wind} label="CO₂e avoided" value="82.4" unit="t" delta="+9.6%" up
                sub="vs. conventional baseline (Higg MSI)" accent="bg-[#D9A441]/15 text-[#9c7220]" />
              <Stat icon={Droplets} label="Water saved" value="3.1" unit="M L" delta="+14.8%" up
                sub="Closed-loop dyeing across 4 mills" accent="bg-[#3a6b8a]/10 text-[#3a6b8a]" />
              <Stat icon={Recycle} label="Landfill diversion" value="96.2" unit="%" delta="−0.4%" up={false}
                sub="312 kg offcuts pending re-spin" accent="bg-[#C2603D]/10 text-[#C2603D]" />
            </section>

            {/* MIDDLE ROW */}
            <section className="grid grid-cols-1 xl:grid-cols-3 gap-5">
              {/* Revenue vs impact */}
              <div className="xl:col-span-2 bg-[#FBF8F1] border border-[#E2DBC9] rounded-[4px] p-6">
                <div className="flex flex-wrap items-center gap-3 mb-1">
                  <div>
                    <h2 className="font-fraunces text-[20px]">Revenue grows, footprint doesn't</h2>
                    <p className="text-[12px] text-[#8A8472] mt-0.5">Net revenue against CO₂e avoided · decoupling index 2.3×</p>
                  </div>
                  <div className="ml-auto flex bg-[#EDE8DB] rounded-full p-0.5">
                    {['3M', '8M', 'YTD'].map(r => (
                      <button key={r} onClick={() => setRange(r)}
                        className={`px-3 py-1 text-[12px] rounded-full transition-all ${range === r ? 'bg-[#1C2B22] text-[#F4F1EA] font-medium' : 'text-[#6E6A5C] hover:text-[#1C2B22]'}`}>
                        {r}
                      </button>
                    ))}
                  </div>
                </div>
                <div className="flex items-center gap-5 mt-3 mb-2 text-[12px] text-[#6E6A5C]">
                  <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-full bg-[#3F5C46]" />Net revenue ($k)</span>
                  <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-full bg-[#C2603D]" />CO₂e avoided (t)</span>
                </div>
                <div className="h-[265px] -ml-2">
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart data={revenueImpact}>
                      <defs>
                        <linearGradient id="rev" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="0%" stopColor="#3F5C46" stopOpacity={0.28} />
                          <stop offset="100%" stopColor="#3F5C46" stopOpacity={0} />
                        </linearGradient>
                        <linearGradient id="co2" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="0%" stopColor="#C2603D" stopOpacity={0.22} />
                          <stop offset="100%" stopColor="#C2603D" stopOpacity={0} />
                        </linearGradient>
                      </defs>
                      <CartesianGrid stroke="#E2DBC9" strokeDasharray="2 6" vertical={false} />
                      <XAxis dataKey="m" tick={{ fill: '#8A8472', fontSize: 12 }} axisLine={false} tickLine={false} dy={8} />
                      <YAxis tick={{ fill: '#8A8472', fontSize: 12 }} axisLine={false} tickLine={false} width={36} />
                      <Tooltip content={<ChartTip />} cursor={{ stroke: '#1C2B22', strokeOpacity: 0.15 }} />
                      <Area type="monotone" dataKey="revenue" name="Revenue" stroke="#3F5C46" strokeWidth={2.2} fill="url(#rev)" />
                      <Area type="monotone" dataKey="co2" name="CO₂e avoided" stroke="#C2603D" strokeWidth={2.2} fill="url(#co2)" />
                    </AreaChart>
                  </ResponsiveContainer>
                </div>
              </div>

              {/* Materials */}
              <div className="bg-[#1C2B22] text-[#E9E4D5] rounded-[4px] p-6 flex flex-col">
                <h2 className="font-fraunces text-[20px]">Fiber composition</h2>
                <p className="text-[12px] text-[#8A9B7C] mt-0.5">FW25 collection · by garment weight</p>
                <div className="relative h-[190px] mt-2">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie data={materials} dataKey="value" innerRadius={62} outerRadius={86} paddingAngle={3} stroke="none" startAngle={90} endAngle={-270}>
                        {materials.map((m) => <Cell key={m.name} fill={m.color} />)}
                      </Pie>
                    </PieChart>
                  </ResponsiveContainer>
                  <div className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none">
                    <p className="font-fraunces text-[30px] leading-none">100%</p>
                    <p className="text-[10px] uppercase tracking-[0.16em] text-[#8A9B7C] mt-1">preferred fibers</p>
                  </div>
                </div>
                <div className="mt-3 space-y-2.5">
                  {materials.map(m => (
                    <div key={m.name} className="flex items-center text-[13px]">
                      <span className="w-2.5 h-2.5 rounded-full mr-2.5" style={{ background: m.color }} />
                      <span className="text-[#D8D2C0]">{m.name}</span>
                      <span className="ml-auto font-semibold">{m.value}%</span>
                    </div>
                  ))}
                </div>
                <button className="mt-auto pt-4 flex items-center gap-1.5 text-[12px] text-[#D9A441] hover:gap-2.5 transition-all">
                  Open material passport <ArrowUpRight size={13} />
                </button>
              </div>
            </section>

            {/* BOTTOM ROW */}
            <section className="grid grid-cols-1 xl:grid-cols-5 gap-5">
              {/* Top products */}
              <div className="xl:col-span-2 bg-[#FBF8F1] border border-[#E2DBC9] rounded-[4px] p-6">
                <div className="flex items-center justify-between">
                  <div>
                    <h2 className="font-fraunces text-[20px]">Best sellers by impact score</h2>
                    <p className="text-[12px] text-[#8A8472] mt-0.5">Eco-score blends LCA, durability & repairability</p>
                  </div>
                  <Package size={18} className="text-[#8A8472]" />
                </div>
                <div className="mt-4 divide-y divide-[#EDE6D6]">
                  {products.map((p, i) => (
                    <div key={p.name} className="flex items-center gap-3.5 py-3 group cursor-pointer">
                      <span className="font-fraunces text-[15px] text-[#B0A88F] w-4">{i + 1}</span>
                      <img src={p.img} alt="" className="w-11 h-11 rounded-[3px] object-cover" />
                      <div className="min-w-0">
                        <p className="text-[13.5px] font-semibold truncate group-hover:text-[#C2603D] transition-colors">{p.name}</p>
                        <p className="text-[12px] text-[#8A8472]">{p.cat} · {p.units.toLocaleString()} units</p>
                      </div>
                      <div className="ml-auto flex items-center gap-3">
                        <span className={`text-[12px] font-medium ${p.trend >= 0 ? 'text-[#3F5C46]' : 'text-[#C2603D]'}`}>
                          {p.trend >= 0 ? '+' : ''}{p.trend}%
                        </span>
                        <span className="text-[12px] font-bold bg-[#3F5C46]/10 text-[#3F5C46] rounded-full px-2.5 py-1">{p.score}</span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              {/* Recommerce */}
              <div className="xl:col-span-2 bg-[#FBF8F1] border border-[#E2DBC9] rounded-[4px] p-6 flex flex-col">
                <div className="flex items-center justify-between">
                  <div>
                    <h2 className="font-fraunces text-[20px]">Second-life program</h2>
                    <p className="text-[12px] text-[#8A8472] mt-0.5">Resale & repair volume · Tellure Re:Worn</p>
                  </div>
                  <span className="text-[11px] font-semibold uppercase tracking-wide bg-[#C2603D]/10 text-[#C2603D] px-2.5 py-1 rounded-full">+38% QoQ</span>
                </div>
                <div className="h-[210px] mt-4 -ml-2">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={recommerce} barGap={4}>
                      <CartesianGrid stroke="#E2DBC9" strokeDasharray="2 6" vertical={false} />
                      <XAxis dataKey="m" tick={{ fill: '#8A8472', fontSize: 12 }} axisLine={false} tickLine={false} dy={6} />
                      <YAxis tick={{ fill: '#8A8472', fontSize: 12 }} axisLine={false} tickLine={false} width={32} />
                      <Tooltip content={<ChartTip />} cursor={{ fill: '#1C2B22', opacity: 0.04 }} />
                      <Bar dataKey="resold" name="Resold" fill="#3F5C46" radius={[3, 3, 0, 0]} maxBarSize={22} />
                      <Bar dataKey="repaired" name="Repaired" fill="#D9A441" radius={[3, 3, 0, 0]} maxBarSize={22} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
                <div className="mt-auto grid grid-cols-2 gap-3 pt-3">
                  <div className="bg-[#EDE8DB] rounded-[4px] px-4 py-3">
                    <p className="text-[11px] uppercase tracking-[0.12em] text-[#6E6A5C]">Resale revenue</p>
                    <p className="font-fraunces text-[22px] mt-0.5">$48.2k</p>
                  </div>
                  <div className="bg-[#EDE8DB] rounded-[4px] px-4 py-3">
                    <p className="text-[11px] uppercase tracking-[0.12em] text-[#6E6A5C]">Avg. repair turnaround</p>
                    <p className="font-fraunces text-[22px] mt-0.5">6.4 days</p>
                  </div>
                </div>
              </div>

              {/* Supply chain */}
              <div className="bg-[#FBF8F1] border border-[#E2DBC9] rounded-[4px] p-6">
                <div className="flex items-center justify-between">
                  <h2 className="font-fraunces text-[20px]">Traceability</h2>
                  <Globe2 size={18} className="text-[#8A8472]" />
                </div>

                {/* ring */}
                <div className="flex items-center gap-4 mt-4 pb-5 border-b border-[#EDE6D6]">
                  <svg width="92" height="92" viewBox="0 0 100 100" className="-rotate-90">
                    <circle cx="50" cy="50" r="42" fill="none" strokeWidth="9" className="ring-track" />
                    <circle cx="50" cy="50" r="42" fill="none" strokeWidth="9" stroke="#3F5C46" strokeLinecap="round"
                      strokeDasharray="264" strokeDashoffset={264 - 264 * 0.87} className="ring-fill" />
                  </svg>
                  <div>
                    <p className="font-fraunces text-[28px] leading-none">87%</p>
                    <p className="text-[12px] text-[#8A8472] mt-1 leading-snug">of FW25 SKUs traced to Tier 3 (raw fiber)</p>
                  </div>
                </div>

                <div className="mt-4 space-y-4">
                  {suppliers.map(s => (
                    <div key={s.name} className="group cursor-pointer">
                      <div className="flex items-center gap-2">
                        <p className="text-[13.5px] font-semibold group-hover:text-[#C2603D] transition-colors">{s.name}</p>
                        <span className="ml-auto text-[11px] font-bold bg-[#1C2B22] text-[#F4F1EA] rounded-full px-2 py-[2px]">{s.audit}</span>
                      </div>
                      <p className="text-[12px] text-[#8A8472] flex items-center gap-1 mt-0.5">
                        <MapPin size={11} /> {s.loc} · {s.tier}
                      </p>
                      <div className="flex items-center gap-2 mt-1.5">
                        <div className="h-[5px] flex-1 rounded-full bg-[#EDE8DB] overflow-hidden">
                          <div className="h-full bg-[#8A9B7C] rounded-full" style={{ width: `${s.share * 2.4}%` }} />
                        </div>
                        <span className="text-[11px] text-[#6E6A5C] w-16 text-right">{s.share}% vol.</span>
                      </div>
                      <p className="text-[11px] text-[#A39B86] mt-1">{s.status}</p>
                    </div>
                  ))}
                </div>
              </div>
            </section>

            <footer className="pb-6 flex flex-wrap items-center gap-2 text-[12px] text-[#A39B86]">
              <Leaf size={13} className="text-[#3F5C46]" />
              Tellure Studio OS · Impact methodology v4.2 (verified by Carbonfact) · Last sync 7 minutes ago
              <span className="ml-auto">© 2025 Tellure Apparel Co-op</span>
            </footer>
          </div>
        </main>
      </div>
    </div>
  );
}