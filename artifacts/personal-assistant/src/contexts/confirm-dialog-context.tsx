import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react';
import { IconAlertTriangle, IconLogout, IconTrash } from '@tabler/icons-react';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';

export type ConfirmDialogOptions = {
  title: string;
  description: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'quick' | 'icon' | 'destructive' | 'nested' | 'logout';
};

type ConfirmDialogContextValue = { confirm: (options: ConfirmDialogOptions) => Promise<boolean> };
const ConfirmDialogContext = createContext<ConfirmDialogContextValue | null>(null);

export function ConfirmDialogProvider({ children }: { children: ReactNode }) {
  const [request, setRequest] = useState<ConfirmDialogOptions | null>(null);
  const resolveRef = useRef<((confirmed: boolean) => void) | null>(null);
  const settle = useCallback((confirmed: boolean) => {
    const resolve = resolveRef.current;
    resolveRef.current = null;
    resolve?.(confirmed);
    setRequest(null);
  }, []);
  const confirm = useCallback((options: ConfirmDialogOptions) => new Promise<boolean>((resolve) => {
    if (resolveRef.current) resolveRef.current(false);
    resolveRef.current = resolve;
    setRequest(options);
  }), []);
  const variant = request?.variant ?? 'quick';
  const Icon = variant === 'logout' ? IconLogout : variant === 'quick' ? null : variant === 'nested' ? IconAlertTriangle : IconTrash;

  return (
    <ConfirmDialogContext.Provider value={{ confirm }}>
      {children}
      <AlertDialog
        open={request !== null}
        onOpenChange={(open) => {
          if (!open) settle(false);
        }}
      >
        <AlertDialogContent size={variant === 'destructive' ? 'default' : 'sm'}>
          <AlertDialogHeader className={variant === 'logout' ? 'place-items-center text-center' : undefined}>
            {Icon && (
              <AlertDialogMedia className={variant === 'destructive' ? 'bg-destructive/10 text-destructive' : undefined}>
                <Icon aria-hidden="true" />
              </AlertDialogMedia>
            )}
            <AlertDialogTitle>{request?.title}</AlertDialogTitle>
            <AlertDialogDescription>{request?.description}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter className={variant === 'logout' ? 'grid grid-cols-2 gap-0 overflow-hidden rounded-xl border divide-x' : undefined}>
            <AlertDialogCancel
              size="sm"
              onClick={() => settle(false)}
              className={variant === 'logout' ? 'rounded-none border-0' : undefined}
            >
              {request?.cancelLabel ?? 'Cancel'}
            </AlertDialogCancel>
            <AlertDialogAction
              size="sm"
              variant={variant === 'destructive' ? 'destructive' : 'default'}
              onClick={() => settle(true)}
              className={variant === 'logout' ? 'rounded-none' : undefined}
            >
              {request?.confirmLabel ?? 'Continue'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </ConfirmDialogContext.Provider>
  );
}

export function useConfirmDialog() {
  const context = useContext(ConfirmDialogContext);
  if (!context) throw new Error('useConfirmDialog must be used within ConfirmDialogProvider');
  return context;
}
