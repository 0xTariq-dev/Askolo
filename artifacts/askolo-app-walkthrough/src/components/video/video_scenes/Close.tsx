import { motion } from 'framer-motion';
import { useState } from 'react';
import { useSceneTimer } from '@/lib/video';
import { ImageFrame, Mark, Shot } from './Visual';

export function Close() {
  const [beat, setBeat] = useState(0);
  useSceneTimer([
    { time: 1500, callback: () => setBeat(1) },
    { time: 3100, callback: () => setBeat(2) },
    { time: 5200, callback: () => setBeat(3) },
    { time: 7000, callback: () => setBeat(4) },
  ]);
  return (
    <Shot style="resolve">
      <motion.div className="absolute inset-x-[10%] top-[18%] h-[46%] flex gap-[2vmin]"
        initial={{ opacity: 1, scale: 1.09 }}
        animate={{ opacity: beat >= 2 ? 0 : 1, scale: beat >= 2 ? .75 : 1 }}
        transition={{ duration: .8 }}>
        {['dashboard', 'planner', 'actions'].map(image => <ImageFrame key={image} image={image} className="w-1/3 h-full" />)}
      </motion.div>
      <motion.div className="absolute inset-0 flex flex-col items-center justify-center text-center"
        initial={{ opacity: 0, scale: .85 }}
        animate={{ opacity: beat >= 2 ? 1 : 0, scale: beat >= 2 ? 1 : .85 }}
        transition={{ duration: .85 }}>
        <Mark className="h-[19vmin] w-[19vmin] object-contain" />
        <h2 className="headline mt-[2vmin]" style={{ fontSize: '7.8vmin' }}>Your whole life, one place</h2>
        <motion.div className="rule mt-[4vmin]" initial={{ width: 0 }} animate={{ width: beat >= 3 ? '13vmin' : 0 }} />
        <motion.p className="subline mt-[3vmin]" initial={{ opacity: 0, y: 15 }} animate={{ opacity: beat >= 3 ? 1 : 0, y: beat >= 3 ? 0 : 15 }}>
          Plan by voice. Stay in control.
        </motion.p>
      </motion.div>
      <motion.div className="absolute bottom-[7%] left-1/2 h-[.5vmin] bg-[#9CC4F3] rounded-full"
        initial={{ width: 0, x: '-50%' }} animate={{ width: beat >= 4 ? '12vmin' : 0 }}
        transition={{ duration: .55 }} />
    </Shot>
  );
}