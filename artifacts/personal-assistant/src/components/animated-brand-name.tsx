import { motion, useReducedMotion } from 'framer-motion';

const brandLetters = ['A', 'S', 'K', 'O', 'L', 'O'];

export function AnimatedBrandName({
  layoutIdPrefix,
  className = '',
}: {
  layoutIdPrefix?: string;
  className?: string;
}) {
  const reduceMotion = useReducedMotion() ?? false;

  return (
    <span className={`inline-flex items-baseline font-display font-bold tracking-tight text-foreground transition-colors group-hover:text-primary ${className}`}>
      <span className="sr-only">Askolo</span>
      <span aria-hidden="true" className="inline-flex">
        {brandLetters.map((letter, index) => (
          <motion.span
            key={`${letter}-${index}`}
            layoutId={layoutIdPrefix ? `${layoutIdPrefix}-letter-${index}` : undefined}
            initial={reduceMotion || layoutIdPrefix ? false : { y: 8, scale: 0.9 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            transition={
              reduceMotion
                ? { duration: 0 }
                : layoutIdPrefix
                  ? { duration: 0.75, ease: 'easeInOut' }
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