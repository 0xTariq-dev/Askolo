import { createElement, type ComponentProps, type ComponentType } from 'react';
import * as LucideIcons from 'lucide-react';

type Props = {
  lucide?: string;
  tabler?: string;
  hugeicons?: string;
  phosphor?: string;
  remixicon?: string;
  className?: string;
} & Omit<ComponentProps<'svg'>, 'ref'>;

const icons = LucideIcons as unknown as Record<string, ComponentType<ComponentProps<'svg'>>>;

export function IconPlaceholder({ lucide, tabler: _tabler, hugeicons: _hugeicons, phosphor: _phosphor, remixicon: _remixicon, ...props }: Props) {
  const name = lucide?.replace(/Icon$/, '') ?? 'Sparkles';
  const Icon = icons[name] ?? LucideIcons.Sparkles;
  return createElement(Icon, props);
}
