import { toast as sonnerToast } from 'sonner';
type ToastOptions = { title?: React.ReactNode; description?: React.ReactNode; variant?: 'default'|'destructive'; };
export function useToast() {
  return { toast: ({ title, description, variant }: ToastOptions) => variant === 'destructive' ? sonnerToast.error(title, { description }) : sonnerToast(title, { description }) };
}