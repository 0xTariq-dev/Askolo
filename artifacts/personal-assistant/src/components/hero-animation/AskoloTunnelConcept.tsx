import { motion } from 'framer-motion';
import type { ThemeMode } from '@/lib/theme';

interface AskoloTunnelConceptProps {
  theme: ThemeMode;
  reducedMotion: boolean;
  paused?: boolean;
}

const rings = [
  { rx: 530, ry: 252, rotate: -8 },
  { rx: 445, ry: 210, rotate: 7 },
  { rx: 352, ry: 166, rotate: -6 },
  { rx: 254, ry: 120, rotate: 8 },
  { rx: 152, ry: 73, rotate: -7 },
];

export function AskoloTunnelConcept({
  theme,
  reducedMotion,
  paused = false,
}: AskoloTunnelConceptProps) {
  const accent = theme === 'dark' ? '#73c9ee' : '#176fa7';
  const warmAccent = theme === 'dark' ? '#ffc46d' : '#bf6c24';
  const animate = !reducedMotion && !paused;

  return (
    <div className="absolute inset-0 overflow-hidden" aria-hidden="true">
      <motion.svg
        viewBox="0 0 1440 900"
        preserveAspectRatio="xMidYMid slice"
        className="h-full w-full"
        role="presentation"
        initial={false}
        animate={animate ? { opacity: [0.86, 1, 0.86] } : { opacity: 1 }}
        transition={animate ? { duration: 8, repeat: Infinity, ease: 'easeInOut' } : undefined}
      >
        <defs>
          <radialGradient id="askolo-tunnel-glow">
            <stop offset="0%" stopColor={warmAccent} stopOpacity="0.28" />
            <stop offset="48%" stopColor={accent} stopOpacity="0.13" />
            <stop offset="100%" stopColor={accent} stopOpacity="0" />
          </radialGradient>
          <linearGradient id="askolo-tunnel-line" x1="0" x2="1" y1="0" y2="1">
            <stop offset="0%" stopColor={accent} stopOpacity="0.08" />
            <stop offset="52%" stopColor={accent} stopOpacity="0.62" />
            <stop offset="100%" stopColor={warmAccent} stopOpacity="0.23" />
          </linearGradient>
        </defs>

        <ellipse cx="720" cy="455" rx="660" ry="380" fill="url(#askolo-tunnel-glow)" />
        <g fill="none" stroke="url(#askolo-tunnel-line)" strokeWidth="1.4">
          {rings.map((ring, index) => (
            <motion.ellipse
              key={`${ring.rx}-${ring.ry}`}
              cx="720"
              cy="455"
              rx={ring.rx}
              ry={ring.ry}
              transform={`rotate(${ring.rotate} 720 455)`}
              initial={false}
              animate={animate ? { scale: [1, 1.018, 1] } : { scale: 1 }}
              transition={
                animate
                  ? {
                      duration: 7 + index * 0.8,
                      repeat: Infinity,
                      ease: 'easeInOut',
                      delay: index * 0.18,
                    }
                  : undefined
              }
              style={{ transformOrigin: '720px 455px' }}
            />
          ))}
          <path d="M0 455 C280 348 470 330 720 455 C970 580 1160 562 1440 455" />
          <path d="M0 500 C290 400 484 386 720 500 C956 614 1150 600 1440 500" opacity=".45" />
          <path d="M0 410 C280 310 470 294 720 410 C970 526 1160 510 1440 410" opacity=".45" />
        </g>
        <circle cx="720" cy="455" r="7" fill={warmAccent} fillOpacity=".88" />
        <circle cx="720" cy="455" r="20" fill="none" stroke={warmAccent} strokeOpacity=".46" />
      </motion.svg>
    </div>
  );
}