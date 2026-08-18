import { lazy, Suspense, useState, useEffect } from 'react';
import { ToastProvider } from '@/components/ui/Toast';
import { ErrorBoundary } from '@/components/ui/ErrorBoundary';
import { Sidebar, type NavTab } from '@/components/layout/Sidebar';
import { Header } from '@/components/layout/Header';
import { SetupWizardPage } from '@/pages/Auth/SetupWizardPage';
import { LoginPage } from '@/pages/Auth/LoginPage';
import { api } from '@/lib/api';
import { ApprovalInterruption } from '@/components/features/governance/ApprovalInterruption';
import { RealtimeProvider } from '@/components/providers/RealtimeProvider';
import { DensityProvider } from '@/components/providers/DensityProvider';
import { useTranslation } from 'react-i18next';
import { CommandPalette } from '@/components/features/search/CommandPalette';

const DashboardPage = lazy(() => import('@/pages/Dashboard/DashboardPage').then((m) => ({ default: m.DashboardPage })));
const AgentsPage = lazy(() => import('@/pages/Agents/AgentsPage').then((m) => ({ default: m.AgentsPage })));
const AgentStudioPage = lazy(() => import('@/pages/Agents/AgentStudioPage').then((m) => ({ default: m.AgentStudioPage })));
const ChatPage = lazy(() => import('@/pages/Chat/ChatPage').then((m) => ({ default: m.ChatPage })));
const MissionsPage = lazy(() => import('@/pages/Missions/MissionsPage').then((m) => ({ default: m.MissionsPage })));
const OperationsPage = lazy(() => import('@/pages/Operations/OperationsPage').then((m) => ({ default: m.OperationsPage })));
const AutomationsPage = lazy(() => import('@/pages/Automations/AutomationsPage').then((m) => ({ default: m.AutomationsPage })));
const ChannelsPage = lazy(() => import('@/pages/Channels/ChannelsPage').then((m) => ({ default: m.ChannelsPage })));
const ConnectorsPage = lazy(() => import('@/pages/Connectors/ConnectorsPage').then((m) => ({ default: m.ConnectorsPage })));
const ToolHubPage = lazy(() => import('@/pages/ToolHub/ToolHubPage').then((m) => ({ default: m.ToolHubPage })));
const SkillsPage = lazy(() => import('@/pages/Skills/SkillsPage').then((m) => ({ default: m.SkillsPage })));
const WorkspacePage = lazy(() => import('@/pages/Workspace/WorkspacePage').then((m) => ({ default: m.WorkspacePage })));
const SettingsPage = lazy(() => import('@/pages/Settings/SettingsPage').then((m) => ({ default: m.SettingsPage })));

export const navTabs: NavTab[] = [
  'dashboard', 'agents', 'agent-studio', 'chat', 'missions', 'operations',
  'automations', 'tools', 'skills', 'workspace', 'channels', 'connectors', 'settings',
];

export function tabFromLocation(): NavTab {
  const value = window.location.hash.replace(/^#\/?/, '').split('?')[0];
  if (value === 'agents/new' || value.startsWith('agents/')) return 'agent-studio';
  return navTabs.includes(value as NavTab) ? value as NavTab : 'dashboard';
}

export function App() {
  const { t } = useTranslation('common');
  const [authStatus, setAuthStatus] = useState<{
    loading: boolean;
    initialized: boolean;
    authenticated: boolean;
    userName?: string;
  }>({
    loading: true,
    initialized: false,
    authenticated: false,
  });

  const [activeTab, setActiveTab] = useState<NavTab>(tabFromLocation);
  const [selectedAgentID, setSelectedAgentID] = useState<string>('agent_system_core');
  const [studioAgentID, setStudioAgentID] = useState<string>('agent_system_core');
  const [collapsed, setCollapsed] = useState<boolean>(() => {
    return localStorage.getItem('actonos_sidebar_collapsed') === 'true';
  });
  const [mobileOpen, setMobileOpen] = useState(false);
  const [commandOpen, setCommandOpen] = useState(false);

  const checkAuth = async () => {
    try {
      const res = await api.getAuthStatus();
      setAuthStatus({
        loading: false,
        initialized: res.initialized,
        authenticated: res.authenticated,
        userName: res.user_name,
      });
    } catch {
      setAuthStatus({
        loading: false,
        initialized: true,
        authenticated: false,
      });
    }
  };

  useEffect(() => {
    checkAuth();
  }, []);

  useEffect(() => {
    const syncLocation = () => {
      const nextTab = tabFromLocation();
      const route = window.location.hash.replace(/^#\/?/, '').split('?')[0];
      if (nextTab === 'agent-studio' && route.startsWith('agents/')) {
        setStudioAgentID(decodeURIComponent(route.slice('agents/'.length)) || 'new');
      }
      setActiveTab(nextTab);
    };
    window.addEventListener('hashchange', syncLocation);
    return () => window.removeEventListener('hashchange', syncLocation);
  }, []);

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLocaleLowerCase() === 'k') {
        event.preventDefault();
        setCommandOpen((current) => !current);
      }
    };
    window.addEventListener('keydown', handleShortcut);
    return () => window.removeEventListener('keydown', handleShortcut);
  }, []);

  const navigateTab = (tab: NavTab) => {
    if (window.location.hash !== `#/${tab}`) window.location.hash = `/${tab}`;
    setActiveTab(tab);
  };

  const handleLogout = async () => {
    try {
      await api.logout();
    } catch {}
    setAuthStatus((prev) => ({ ...prev, authenticated: false }));
  };

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
    navigateTab('chat');
  };

  const handleEditAgent = (agentID: string) => {
    setStudioAgentID(agentID);
    window.location.hash = `/agents/${encodeURIComponent(agentID)}`;
    setActiveTab('agent-studio');
  };

  if (authStatus.loading) {
    return (
      <div className="min-h-screen bg-canvas flex items-center justify-center font-sans text-slate">
        <div className="flex items-center gap-3 text-body-sm font-medium">
          <div className="w-4 h-4 border-2 border-deep-ink border-t-transparent rounded-full animate-spin" />
          <span>{t('startup')}</span>
        </div>
      </div>
    );
  }

  if (!authStatus.initialized) {
    return (
      <ToastProvider>
        <SetupWizardPage onCompleted={checkAuth} />
      </ToastProvider>
    );
  }

  if (!authStatus.authenticated) {
    return (
      <ToastProvider>
        <LoginPage userName={authStatus.userName} onAuthenticated={checkAuth} />
      </ToastProvider>
    );
  }

  return (
    <ToastProvider>
      <DensityProvider>
      <RealtimeProvider>
      <div className="min-h-screen bg-canvas text-deep-ink selection:bg-hi-yellow selection:text-deep-ink font-sans flex">
        <ApprovalInterruption />
        <CommandPalette
          isOpen={commandOpen}
          onClose={() => setCommandOpen(false)}
          onNavigate={navigateTab}
          onOpenChat={handleOpenChatWithAgent}
          onEditAgent={handleEditAgent}
        />
        {/* Sleek Collapsible Left Sidebar */}
        <Sidebar
          activeTab={activeTab}
          onSelectTab={navigateTab}
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
            onLogout={handleLogout}
            onOpenSearch={() => setCommandOpen(true)}
          />

          {/* Page Views */}
          <main className="flex-1 w-full pb-12">
            <ErrorBoundary>
              <Suspense fallback={<div className="m-8 h-8 w-8 animate-spin rounded-full border-2 border-deep-ink border-t-transparent" />}>
              {activeTab === 'dashboard' && (
                <DashboardPage
                  onNavigateTab={navigateTab}
                  onOpenChat={handleOpenChatWithAgent}
                  onEditAgent={handleEditAgent}
                />
              )}
              {activeTab === 'agents' && (
                <AgentsPage
                  onOpenChat={handleOpenChatWithAgent}
                  onNavigateTab={navigateTab}
                  onEditAgent={handleEditAgent}
                />
              )}
              {activeTab === 'agent-studio' && (
                <AgentStudioPage
                  agentID={studioAgentID}
                  onBack={() => navigateTab('agents')}
                  onOpenChat={handleOpenChatWithAgent}
                />
              )}
              {activeTab === 'chat' && (
                <ChatPage
                  selectedAgentID={selectedAgentID}
                  onSelectAgentID={setSelectedAgentID}
                  onNavigateTab={navigateTab}
                />
              )}
              {activeTab === 'missions' && (
                <MissionsPage
                  onOpenChat={handleOpenChatWithAgent}
                />
              )}
              {activeTab === 'operations' && <OperationsPage />}
              {activeTab === 'automations' && <AutomationsPage />}
              {activeTab === 'channels' && <ChannelsPage />}
              {activeTab === 'connectors' && <ConnectorsPage />}
              {activeTab === 'tools' && <ToolHubPage />}
              {activeTab === 'skills' && <SkillsPage />}
              {activeTab === 'workspace' && <WorkspacePage />}
              {activeTab === 'settings' && <SettingsPage />}
              </Suspense>
            </ErrorBoundary>
          </main>
        </div>
      </div>
      </RealtimeProvider>
      </DensityProvider>
    </ToastProvider>
  );
}

export default App;
