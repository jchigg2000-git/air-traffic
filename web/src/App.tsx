import { Routes, Route, Navigate } from 'react-router-dom'
import FlightDeck from './pages/FlightDeck.tsx'
import Layout from './components/Layout.tsx'
import RigorConsole from './pages/RigorConsole.tsx'
import PolicyEditor from './pages/PolicyEditor.tsx'
import CostExplorer from './pages/CostExplorer.tsx'
import Vendors from './pages/Vendors.tsx'
import Observability from './pages/Observability.tsx'
import Audit from './pages/Audit.tsx'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<FlightDeck />} />
      <Route path="/settings" element={<Layout />}>
        <Route index element={<Navigate to="/settings/rigor" replace />} />
        <Route path="rigor" element={<RigorConsole />} />
        <Route path="policy" element={<PolicyEditor />} />
        <Route path="cost" element={<CostExplorer />} />
        <Route path="vendors" element={<Vendors />} />
        <Route path="observability" element={<Observability />} />
        <Route path="audit" element={<Audit />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
