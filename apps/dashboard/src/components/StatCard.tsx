import { Card } from '@mc/ui';
import { motion } from 'framer-motion';
import type { LucideIcon } from 'lucide-react';

interface StatCardProps {
  label: string;
  value: number | string;
  icon: LucideIcon;
  accent?: string;
}

export function StatCard({ label, value, icon: Icon, accent = 'text-indigo-400' }: StatCardProps) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2 }}
    >
      <Card className="p-5">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wide text-zinc-500">{label}</span>
          <Icon className={`h-4 w-4 ${accent}`} />
        </div>
        <div className="mt-3 text-3xl font-semibold tabular-nums text-zinc-100">{value}</div>
      </Card>
    </motion.div>
  );
}
