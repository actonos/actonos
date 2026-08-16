import { useState } from 'react';
import { Navbar, type NavTab } from '@/components/layout/Navbar';
import { AgentsPage } from '@/pages/Agents/AgentsPage';
import { ToolHubPage } from '@/pages/ToolHub/ToolHubPage';
import { WorkspacePage } from '@/pages/Workspace/WorkspacePage';
import { IntegrationsPage } from '@/pages/Integrations/IntegrationsPage';
import { ChatPage } from '@/pages/Chat/ChatPage';
import { SettingsPage } from '@/pages/Settings/SettingsPage';

export function App() {
  const [activeTab, setActiveTab] = useState<NavTab>('agents');
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);

  return (
    <div className="min-h-screen bg-canvas text-deep-ink selection:bg-hi-yellow selection:text-deep-ink">
      <Navbar
        activeTab={activeTab}
        onSelectTab={setActiveTab}
        onCreateAgent={() => {
          setActiveTab('agents');
          setIsCreateModalOpen(true);
        }}
      />

      <main className="w-full">
        {activeTab === 'agents' && (
          <AgentsPage
            onOpenChat={() => setActiveTab('chat')}
            isCreateModalOpen={isCreateModalOpen}
            setIsCreateModalOpen={setIsCreateModalOpen}
          />
        )}
        {activeTab === 'tools' && <ToolHubPage />}
        {activeTab === 'workspace' && <WorkspacePage />}
        {activeTab === 'integrations' && <IntegrationsPage />}
        {activeTab === 'chat' && <ChatPage />}
        {activeTab === 'settings' && <SettingsPage />}
      </main>
    </div>
  );
}

export default App;
