import { useState } from 'react';
import { Sidebar, type NavTab } from '@/components/layout/Sidebar';
import { Header } from '@/components/layout/Header';
import { AgentsPage } from '@/pages/Agents/AgentsPage';
import { ToolHubPage } from '@/pages/ToolHub/ToolHubPage';
import { WorkspacePage } from '@/pages/Workspace/WorkspacePage';
import { IntegrationsPage } from '@/pages/Integrations/IntegrationsPage';
import { ChatPage } from '@/pages/Chat/ChatPage';
import { SettingsPage } from '@/pages/Settings/SettingsPage';

export function App() {
  const [activeTab, setActiveTab] = useState<NavTab>('agents');
  const [selectedAgentID, setSelectedAgentID] = useState<string>('agent_system_core');
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [collapsed, setCollapsed] = useState<boolean>(() => {
    return localStorage.getItem('actonos_sidebar_collapsed') === 'true';
  });
  const [mobileOpen, setMobileOpen] = useState(false);

  const handleToggleCollapse = () => {
    setCollapsed((prev) => {
      const next = !prev;
      localStorage.setItem('actonos_sidebar_collapsed', String(next));
      return next;
    });
  };

  const handleOpenChatWithAgent = (agentID?: string) => {
    if (agentID) {
      setSelectedAgentID(agentID);
    }
    setActiveTab('chat');
  };

  return (
    <div className="min-h-screen bg-canvas text-deep-ink selection:bg-hi-yellow selection:text-deep-ink font-sans flex">
      {/* Sleek Collapsible Left Sidebar */}
      <Sidebar
        activeTab={activeTab}
        onSelectTab={setActiveTab}
        collapsed={collapsed}
        onToggleCollapse={handleToggleCollapse}
        mobileOpen={mobileOpen}
        onCloseMobile={() => setMobileOpen(false)}
      />

      {/* Main Content Area */}
      <div
        className={`flex-1 flex flex-col min-w-0 transition-all duration-200 ease-in-out ${
          collapsed ? 'lg:ml-20' : 'lg:ml-64'
        }`}
      >
        {/* Sticky Top Header */}
        <Header
          activeTab={activeTab}
          onOpenMobileSidebar={() => setMobileOpen(true)}
          collapsed={collapsed}
        />

        {/* Page Views */}
        <main className="flex-1 w-full pb-12">
          {activeTab === 'agents' && (
            <AgentsPage
              onOpenChat={handleOpenChatWithAgent}
              onNavigateTab={setActiveTab}
              isCreateModalOpen={isCreateModalOpen}
              setIsCreateModalOpen={setIsCreateModalOpen}
            />
          )}
          {activeTab === 'chat' && (
            <ChatPage
              selectedAgentID={selectedAgentID}
              onSelectAgentID={setSelectedAgentID}
            />
          )}
          {activeTab === 'tools' && <ToolHubPage />}
          {activeTab === 'workspace' && <WorkspacePage />}
          {activeTab === 'integrations' && <IntegrationsPage />}
          {activeTab === 'settings' && <SettingsPage />}
        </main>
      </div>
    </div>
  );
}

export default App;
