import type { ComponentProps } from 'react';
import { Link } from 'wouter';

type SiteOriginLinkProps = ComponentProps<'a'> & {
  href: string;
};

export function SiteOriginLink({ href, ...props }: SiteOriginLinkProps) {
  const target = new URL(href, window.location.href);

  if (target.origin !== window.location.origin) {
    return <a href={href} {...props} />;
  }

  return (
    <Link
      href={`${target.pathname}${target.search}${target.hash}`}
      {...props}
    />
  );
}