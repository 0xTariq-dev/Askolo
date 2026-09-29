import { motion, useReducedMotion } from 'framer-motion';

const brandLetters = ['A', 'S', 'K', 'O', 'L', 'O'];

export function AnimatedBrandName() {
  const reduceMotion = useReducedMotion() ?? false;

  return (
    <span className="inline-flex items-baseline font-display text-xl font-bold tracking-tight text-foreground transition-colors group-hover:text-primary">
      <span className="sr-only">Askolo</span>
      <span aria-hidden="true" className="inline-flex">
        {brandLetters.map((letter, index) => (
          <motion.span
            key={`${letter}-${index}`}
            initial={reduceMotion ? false : { y: 8, scale: 0.9 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            transition={
              reduceMotion
                ? { duration: 0 }
                : { delay: index * 0.08, duration: 0.3, ease: 'easeOut' }
            }
            className="inline-block"
          >
            {letter}
          </motion.span>
        ))}
      </span>
    </span>
  );
}