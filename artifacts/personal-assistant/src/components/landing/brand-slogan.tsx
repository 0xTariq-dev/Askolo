import { useEffect, useState } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'framer-motion';

interface SloganPart {
  text: string;
  accent?: boolean;
}

interface SloganSegment {
  id: string;
  brandLetters: string;
  parts: SloganPart[];
}

const sloganSegments: SloganSegment[] = [
  {
    id: 'accelerate',
    brandLetters: 'A',
    parts: [
      { text: 'A', accent: true },
      { text: 'ccelerate your progress' },
    ],
  },
  {
    id: 'synchronize',
    brandLetters: 'S',
    parts: [
      { text: 'S', accent: true },
      { text: 'ynchronize your schedule' },
    ],
  },
  {
    id: 'knit',
    brandLetters: 'K',
    parts: [
      { text: 'K', accent: true },
      { text: 'nit your day Together' },
    ],
  },
  {
    id: 'once',
    brandLetters: 'OLO',
    parts: [
      { text: 'You ' },
      { text: 'O', accent: true },
      { text: 'nly ' },
      { text: 'L', accent: true },
      { text: 'ive ' },
      { text: 'O', accent: true },
      { text: 'nce' },
    ],
  },
];

const lettersFromSegment = (segment: SloganSegment) => segment.brandLetters;
const fullBrandName = sloganSegments.map(lettersFromSegment).join('');
const fullSlogan = sloganSegments
  .map((segment) => segment.parts.map(({ text }) => text).join(''))
  .join(' ');

const sloganTextClassName =
  'm-0 block w-full font-display text-2xl font-bold leading-tight tracking-tight text-foreground sm:text-3xl md:text-4xl';
const brandRevealDelayMs = 950;
const brandLetterStaggerMs = 240;
const morphTransition = {
  duration: 0.8,
  ease: [0.25, 0.46, 0.45, 0.94] as const,
  filter: { duration: 0.6 },
  scale: { duration: 0.6 },
  rotateX: { duration: 0.8 },
};

export function BrandSloganReveal() {
  const prefersReducedMotion = useReducedMotion() ?? false;
  const [activeSegmentIndex, setActiveSegmentIndex] = useState(0);
  const [revealedName, setRevealedName] = useState(prefersReducedMotion ? fullBrandName : '');

  useEffect(() => {
    setActiveSegmentIndex(0);

    if (prefersReducedMotion) {
      setRevealedName(fullBrandName);
      return;
    }

    setRevealedName('');
    const revealTimeouts = new Set<number>();
    const clearRevealTimeouts = () => {
      revealTimeouts.forEach((timeout) => window.clearTimeout(timeout));
      revealTimeouts.clear();
    };
    const scheduleNameReveal = (segmentIndex: number) => {
      Array.from(lettersFromSegment(sloganSegments[segmentIndex])).forEach((letter, index) => {
        let timeout = 0;
        timeout = window.setTimeout(
          () => {
            revealTimeouts.delete(timeout);
            setRevealedName((currentName) =>
              currentName.length >= fullBrandName.length
                ? fullBrandName
                : `${currentName}${letter}`,
            );
          },
          brandRevealDelayMs + index * brandLetterStaggerMs,
        );
        revealTimeouts.add(timeout);
      });
    };

    scheduleNameReveal(0);
    let nextIndex = 0;
    const interval = window.setInterval(() => {
      nextIndex = (nextIndex + 1) % sloganSegments.length;
      setActiveSegmentIndex(nextIndex);

      if (nextIndex > 0) {
        clearRevealTimeouts();
        scheduleNameReveal(nextIndex);
      }
    }, 3000);

    return () => {
      window.clearInterval(interval);
      clearRevealTimeouts();
    };
  }, [prefersReducedMotion]);

  const activeSegment = sloganSegments[activeSegmentIndex];

  return (
    <>
      <h1
        id="landing-heading"
        className="min-h-[1.1em] max-w-full text-4xl font-display font-bold tracking-tight sm:text-5xl md:text-6xl lg:text-7xl"
      >
        <span className="sr-only">Askolo</span>
        <span
          aria-hidden="true"
          className="inline-flex items-baseline text-primary"
          style={{ perspective: '900px' }}
        >
          {Array.from(revealedName).map((letter, index) => (
            <motion.span
              key={`brand-letter-${index}`}
              initial={
                prefersReducedMotion
                  ? false
                  : {
                      opacity: 0,
                      filter: 'blur(10px)',
                      scale: 0.8,
                      rotateX: -90,
                    }
              }
              animate={{
                opacity: 1,
                filter: 'blur(0px)',
                scale: 1,
                rotateX: 0,
              }}
              transition={
                prefersReducedMotion
                  ? { duration: 0 }
                  : morphTransition
              }
              className="inline-block"
              style={{ transformStyle: 'preserve-3d' }}
            >
              {letter}
            </motion.span>
          ))}
        </span>
      </h1>

      <div className="mx-auto mb-10 max-w-5xl px-2">
        <div
          aria-hidden="true"
          className="grid min-h-[4em] place-items-center sm:min-h-[3.2em] md:min-h-[2.6em]"
          style={{ perspective: '900px' }}
        >
          {prefersReducedMotion ? (
            <p className={sloganTextClassName}>{fullSlogan}</p>
          ) : (
            <AnimatePresence mode="sync">
              <motion.p
                key={activeSegment.id}
                initial={{
                  opacity: 0,
                  filter: 'blur(10px)',
                  scale: 0.8,
                  rotateX: -90,
                }}
                animate={{
                  opacity: 1,
                  filter: 'blur(0px)',
                  scale: 1,
                  rotateX: 0,
                }}
                exit={{
                  opacity: 0,
                  filter: 'blur(10px)',
                  scale: 1.2,
                  rotateX: 90,
                }}
                transition={morphTransition}
                className={`${sloganTextClassName} col-start-1 row-start-1`}
                style={{ transformStyle: 'preserve-3d' }}
              >
                {activeSegment.parts.map(({ text, accent }, index) =>
                  accent ? (
                    <span key={`${activeSegment.id}-${index}`} className="font-bold text-primary">
                      {text}
                    </span>
                  ) : (
                    <span key={`${activeSegment.id}-${index}`}>{text}</span>
                  ),
                )}
              </motion.p>
            </AnimatePresence>
          )}
        </div>
        <p className="sr-only">{fullSlogan}</p>
      </div>
    </>
  );
}