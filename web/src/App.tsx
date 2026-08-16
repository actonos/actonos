import { useState } from 'react';
import { ToastProvider } from '@/components/ui/Toast';
import { Sidebar, type NavTab } from '@/components/layout/Sidebar';
import { Header } from '@/components/layout/Header';
import { DashboardPage } from '@/pages/Dashboard/DashboardPage';
import { AgentsPage } from '@/pages/Agents/AgentsPage';
import { AgentStudioPage } from '@/pages/Agents/AgentStudioPage';
import { AutomationsPage } from '@/pages/Automations/AutomationsPage';
import { ToolHubPage } from '@/pages/ToolHub/ToolHubPage';
import { SkillsPage } from '@/pages/Skills/SkillsPage';
import { ChannelsPage } from '@/pages/Channels/ChannelsPage';
import { ConnectorsPage } from '@/pages/Connectors/ConnectorsPage';
import { WorkspacePage } from '@/pages/Workspace/WorkspacePage';
import { ChatPage } from '@/pages/Chat/ChatPage';
import { SettingsPage } from '@/pages/Settings/SettingsPage';

export function App() {
  const [activeTab, setActiveTab] = useState<NavTab>('dashboard');
  const [selectedAgentID, setSelectedAgentID] = useState<string>('agent_system_core');
  const [studioAgentID, setStudioAgentID] = useState<string>('agent_system_core');
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

  const handleEditAgent = (agentID: string) => {
    setStudioAgentID(agentID);
    setActiveTab('agent-studio');
  };

  return (
    <ToastProvider>
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
            {activeTab === 'dashboard' && (
              <DashboardPage
                onNavigateTab={setActiveTab}
                onOpenChat={handleOpenChatWithAgent}
                onEditAgent={handleEditAgent}
              />
            )}
            {activeTab === 'agents' && (
              <AgentsPage
                onOpenChat={handleOpenChatWithAgent}
                onNavigateTab={setActiveTab}
                onEditAgent={handleEditAgent}
              />
            )}
            {activeTab === 'agent-studio' && (
              <AgentStudioPage
                agentID={studioAgentID}
                onBack={() => setActiveTab('agents')}
                onOpenChat={handleOpenChatWithAgent}
              />
            )}
            {activeTab === 'chat' && (
              <ChatPage
                selectedAgentID={selectedAgentID}
                onSelectAgentID={setSelectedAgentID}
              />
            )}
            {activeTab === 'automations' && <AutomationsPage />}
            {activeTab === 'channels' && <ChannelsPage />}
            {activeTab === 'connectors' && <ConnectorsPage />}
            {activeTab === 'tools' && <ToolHubPage />}
            {activeTab === 'skills' && <SkillsPage />}
            {activeTab === 'workspace' && <WorkspacePage />}
            {activeTab === 'settings' && <SettingsPage />}
          </main>
        </div>
      </div>
    </ToastProvider>
  );
}

export default App;

