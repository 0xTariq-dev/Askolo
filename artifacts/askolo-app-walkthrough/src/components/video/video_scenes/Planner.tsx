import { motion } from 'framer-motion';
import { useState } from 'react';
import { useSceneTimer } from '@/lib/video';
import { ImageFrame, Shot } from './Visual';

export function Planner() {
  const [beat, setBeat] = useState(0);
  useSceneTimer([
    { time: 2100, callback: () => setBeat(1) },
    { time: 5100, callback: () => setBeat(2) },
    { time: 8000, callback: () => setBeat(3) },
  ]);
  const copy = ['Capture a thought', 'Type or speak', 'AssemblyAI transcribes', 'Review the transcript'][beat];

  return (
    <Shot style="focus">
      <div className="absolute left-[6%] top-[18%] w-[30%]">
        <p className="micro">Daily planner</p>
        <motion.h2 key={copy} className="headline" style={{ fontSize: '7vmin' }}
          initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: .45 }}>
          {copy}
        </motion.h2>
        <p className="subline mt-[5vmin]">Say it when it comes to mind. Review what was heard before moving on.</p>
        <motion.div className="rule mt-[5vmin]" initial={{ width: 0 }} animate={{ width: beat >= 2 ? '26vmin' : '12vmin' }} />
      </div>
      <motion.div className="absolute right-[5%] top-[15%] w-[57%] h-[69%]"
        initial={{ scale: 1.1, y: '5%' }} animate={{ scale: beat >= 1 ? 1.04 : 1, y: beat >= 1 ? '-2%' : 0 }}
        transition={{ duration: 2.2, ease: [0.2, 1, 0.3, 1] }}>
        <ImageFrame image="planner" className="w-full h-full" />
      </motion.div>
      <div className="absolute left-[7%] bottom-[9%] flex items-end gap-[.9vmin]" aria-hidden="true">
        {[2.1, 4, 6.5, 3.4, 7.1, 4.4, 2.8].map((h, i) => (
          <motion.span key={i} className="bg-[#9CC4F3] rounded-full w-[.7vmin]"
            initial={{ height: '1vmin', opacity: .2 }}
            animate={{ height: beat >= 2 ? `${h}vmin` : '1vmin', opacity: beat >= 2 ? .9 : .3 }}
            transition={{ duration: .5, delay: i * .06 }} />
        ))}
      </div>
      <motion.p className="quiet absolute right-[6%] bottom-[5%]" initial={{ opacity: 0 }} animate={{ opacity: beat >= 3 ? 1 : 0 }}>
        The interface shown is an empty state, not a recorded result.
      </motion.p>
    </Shot>
  );
}