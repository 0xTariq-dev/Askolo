import type { PropsWithChildren } from 'react';
import { motion } from 'framer-motion';

const base = import.meta.env.BASE_URL;
export const media = (name: string) => `${base}images/${name}.png`;

type TransitionStyle = 'reveal' | 'split' | 'focus' | 'resolve';

export function Shot({
  children,
  style = 'reveal',
}: PropsWithChildren<{ style?: TransitionStyle }>) {
  const initial = {
    reveal: { opacity: 0, scale: 1.08, clipPath: 'inset(3% 3% 3% 3%)' },
    split: { opacity: 0, scale: 1, clipPath: 'inset(0 45% 0 45%)' },
    focus: { opacity: 0, scale: 1.14, filter: 'blur(12px)' },
    resolve: { opacity: 0, scale: 1.12, clipPath: 'circle(12% at 50% 50%)' },
  }[style];

  return (
    <motion.section
      className="shot"
      initial={initial}
      animate={{ opacity: 1, scale: 1, clipPath: style === 'resolve' ? 'circle(125% at 50% 50%)' : 'inset(0 0 0 0)', filter: 'blur(0px)' }}
      exit={{ opacity: 0, scale: 1.14, filter: 'blur(9px)' }}
      transition={{ duration: 0.65, ease: [0.2, 1, 0.3, 1] }}
    >
      {children}
    </motion.section>
  );
}

export function Mark({ className = '' }: { className?: string }) {
  return <img className={className} src={media('logo')} alt="" />;
}

export function ImageFrame({ image, className = '' }: { image: string; className?: string }) {
  return <div className={`frame ${className}`}><img src={media(image)} alt="" /></div>;
}