import type { CSSProperties } from 'react';
import logoMaskUrl from '/logo.png';
import { cn } from '@/lib/utils';

const maskStyle: CSSProperties = {
  maskImage: `url("${logoMaskUrl}")`,
  maskPosition: 'center',
  maskRepeat: 'no-repeat',
  maskSize: 'contain',
  WebkitMaskImage: `url("${logoMaskUrl}")`,
  WebkitMaskPosition: 'center',
  WebkitMaskRepeat: 'no-repeat',
  WebkitMaskSize: 'contain',
};

export function BrandMark({
  className,
  decorative = false,
}: {
  className?: string;
  decorative?: boolean;
}) {
  return (
    <span
      role={decorative ? undefined : 'img'}
      aria-label={decorative ? undefined : 'Askolo'}
      aria-hidden={decorative || undefined}
      className={cn('inline-block shrink-0 bg-primary', className)}
      style={maskStyle}
    />
  );
}
