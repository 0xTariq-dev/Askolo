import { Button } from '@/components/ui/button';
import { motion } from 'framer-motion';
import { Sparkles, UserPlus } from 'lucide-react';
import { useLocation } from 'wouter';
import logoUrl from '/logo.png';

export function LoginPage() {
  const [, setLocation] = useLocation();

  return (
    <div className="min-h-screen w-full flex bg-background relative overflow-hidden flex-col items-center justify-center p-4">
      {/* Ambient background glows */}
      <div className="absolute top-[0%] left-[-10%] w-[60%] h-[60%] bg-primary/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="absolute bottom-[-10%] right-[-10%] w-[50%] h-[50%] bg-blue-600/10 rounded-full blur-[120px] pointer-events-none" />

      {/* Noise overlay */}
      <div className="absolute inset-0 opacity-[0.03] pointer-events-none bg-[url('https://grainy-gradients.vercel.app/noise.svg')]" />

      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.8, ease: 'easeOut' }}
        className="z-10 flex flex-col items-center max-w-md text-center"
      >
        <div className="h-20 w-20 bg-card border border-white/10 rounded-2xl flex items-center justify-center mb-8 shadow-2xl shadow-black/50 relative overflow-hidden">
          <div className="absolute inset-0 bg-gradient-to-tr from-primary/20 to-transparent opacity-50" />
          <img src={logoUrl} alt="Askolo" className="h-10 w-10 object-contain relative z-10" />
        </div>

        <h1 className="text-4xl md:text-5xl font-display font-bold text-foreground mb-4 tracking-tight">
          Welcome to <span className="text-primary">Askolo</span>
        </h1>

        <p className="text-muted-foreground text-lg mb-10 max-w-sm font-sans">
          Your beautifully designed mission control for habits, goals, and daily focus.
        </p>

        <div className="flex flex-col sm:flex-row gap-4 w-full max-w-xs">
          <Button
            size="lg"
            onClick={() => setLocation('/sign-in')}
            className="flex-1 rounded-full px-8 py-6 text-lg font-medium shadow-[0_0_40px_-10px_rgba(234,179,8,0.3)] hover:shadow-[0_0_60px_-10px_rgba(234,179,8,0.5)] transition-all duration-300 group"
            data-testid="button-login"
          >
            <Sparkles className="mr-2 h-5 w-5 text-primary-foreground/70 group-hover:text-primary-foreground transition-colors" />
            Sign In
          </Button>
          <Button
            size="lg"
            variant="outline"
            onClick={() => setLocation('/sign-up')}
            className="flex-1 rounded-full px-8 py-6 text-lg font-medium border-white/10 hover:bg-white/5 transition-all duration-300 group"
            data-testid="button-signup"
          >
            <UserPlus className="mr-2 h-5 w-5 text-muted-foreground group-hover:text-foreground transition-colors" />
            Get Started
          </Button>
        </div>
      </motion.div>
    </div>
  );
}
