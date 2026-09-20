
import { EntityPage } from '../components/EntityPage';
import { CalibrationGatePanel } from '../components/common/CalibrationGatePanel';
import { ENTITY_CONFIGS } from '../types/status';
import { useColorProofStore } from '../stores/color-proof';

export default function ColorProofPage() {
  return <EntityPage config={ENTITY_CONFIGS[2]} useStore={useColorProofStore} headerPanel={<CalibrationGatePanel />} />;
}
