import { useEffect, useState } from 'react';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { EXTERNAL_EFFECT_TOOLS, mailcalApi } from '@/lib/api/mailcal';

const REFRESH_MS = 60_000;

/**
 * How many mail items wait for the owner: rule proposals plus approvals for tools that leave the
 * device. It carries counts only, never message content.
 */
export function useMailAttention(): number {
  const { snapshot } = useRealtime();
  const [proposals, setProposals] = useState(0);

  useEffect(() => {
    let alive = true;
    const load = () =>
      mailcalApi
        .listProposals('pending')
        .then((r) => alive && setProposals(r.count))
        .catch(() => alive && setProposals(0));
    void load();
    const timer = window.setInterval(() => void load(), REFRESH_MS);
    return () => {
      alive = false;
      window.clearInterval(timer);
    };
  }, []);

  const approvals = (snapshot?.approvals ?? []).filter((a) => EXTERNAL_EFFECT_TOOLS.includes(a.tool_name)).length;
  return proposals + approvals;
}
