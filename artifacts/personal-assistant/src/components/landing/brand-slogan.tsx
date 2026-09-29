import { motion, useReducedMotion } from 'framer-motion';

const brandInitials = ['A', 'S', 'K', 'O', 'L', 'O'];

function SloganWord({ initial, rest }: { initial: string; rest: string }) {
  return (
    <span className="whitespace-nowrap">
      <span className="font-semibold text-primary">{initial.toUpperCase()}</span>
      {rest}
    </span>
  );
}

export function BrandSlogan() {
  const reduceMotion = useReducedMotion() ?? false;

  return (
    <div className="mx-auto max-w-3xl">
      <p
        aria-hidden="true"
        className="text-base leading-relaxed text-muted-foreground sm:text-lg"
      >
        <SloganWord initial="A" rest="ccelerate" /> your progress,{' '}
        <SloganWord initial="S" rest="ynchronize" /> your schedule,{' '}
        <SloganWord initial="K" rest="nit" /> your day together—because you{' '}
        <SloganWord initial="o" rest="nly" /> <SloganWord initial="l" rest="ive" />{' '}
        <SloganWord initial="o" rest="nce" />.
      </p>
      <p className="sr-only">
        Accelerate your progress, Synchronize your schedule, Knit your day
        together—because you only live once.
      </p>
      <div
        aria-hidden="true"
        className="mt-4 flex items-center justify-center"
      >
        {brandInitials.map((letter, index) => (
          <motion.span
            key={`${letter}-${index}`}
            initial={reduceMotion ? false : { opacity: 0, y: 10, scale: 0.85 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            transition={
              reduceMotion
                ? { duration: 0 }
                : { delay: 0.8 + index * 0.14, duration: 0.35, ease: 'easeOut' }
            }
            className="font-display text-xl font-bold tracking-tight text-primary"
          >
            {letter}
          </motion.span>
        ))}
      </div>
    </div>
  );
}