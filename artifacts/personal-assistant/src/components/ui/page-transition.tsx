import { motion, useReducedMotion } from 'framer-motion';
import { ReactNode } from 'react';
import { cn } from '@workspace/askolo-design-system/lib/utils';

export function PageTransition({ children, className }: { children: ReactNode, className?: string }) {
  const reducedMotion = useReducedMotion();
  return (
    <motion.div
      initial={reducedMotion ? false : { opacity: 0, y: 15 }}
      animate={{ opacity: 1, y: 0 }}
      exit={reducedMotion ? undefined : { opacity: 0, y: -15 }}
      transition={reducedMotion ? { duration: 0 } : { duration: 0.4, ease: [0.22, 1, 0.36, 1] }}
      className={cn('relative rounded-2xl border border-border/60 bg-card/20 p-4 shadow-sm sm:p-6', className)}
    >
      {children}
    </motion.div>
  );
}
