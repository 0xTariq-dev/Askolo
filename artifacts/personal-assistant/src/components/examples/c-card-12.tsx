import type { ReactNode } from 'react';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
} from '@/components/ui/card';

export function CCard12AuthCard({
  eyebrow,
  title,
  description,
  children,
  footer,
}: {
  eyebrow: string;
  title: string;
  description?: string;
  children: ReactNode;
  footer?: string;
}) {

  return (
    <Card dir="auto" lang="en" className="mx-auto w-full max-w-md border-border bg-card/80 shadow-xl shadow-black/10 backdrop-blur-sm">
      <CardHeader className="text-start">
        <p className="text-xs font-medium uppercase tracking-wider text-primary">
          {eyebrow}
        </p>
        <h2 className="font-display text-3xl font-semibold leading-tight tracking-tight text-foreground">
          {title}
        </h2>
        {description && (
          <CardDescription className="max-w-sm text-sm leading-relaxed">
            {description}
          </CardDescription>
        )}
      </CardHeader>
      <CardContent className="space-y-5">
        {children}
      </CardContent>
      {footer && (
        <CardFooter className="justify-center border-t border-border/60 pt-4 text-center">
          <p className="w-full text-xs text-muted-foreground">{footer}</p>
        </CardFooter>
      )}
    </Card>
  );
}