import { Hexagon } from 'lucide-react';

export function ComingSoon({ title }: { title: string }) {
  return (
    <div className="h-[80vh] flex flex-col items-center justify-center text-center">
      <div className="h-16 w-16 bg-muted/30 rounded-2xl flex items-center justify-center mb-6">
        <Hexagon className="h-8 w-8 text-primary/50" />
      </div>
      <h2 className="text-2xl font-display font-semibold mb-2">{title}</h2>
      <p className="text-muted-foreground">This section is currently being built.</p>
    </div>
  );
}
