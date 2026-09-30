import * as React from 'react';
import { cn } from '@/lib/utils';
const Field = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => <div className={cn('space-y-2', className)} {...props} />;
const FieldGroup = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => <div className={cn('flex w-full flex-col gap-4', className)} {...props} />;
const FieldLabel = ({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) => <label className={cn('text-sm font-medium leading-none', className)} {...props} />;
const FieldSeparator = ({ className, children, ...props }: React.HTMLAttributes<HTMLDivElement>) => <div className={cn('relative flex items-center gap-2 text-muted-foreground before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border', className)} {...props}>{children}</div>;
export { Field, FieldGroup, FieldLabel, FieldSeparator };