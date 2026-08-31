import { lazy, Suspense, useState, useEffect } from 'react';
import { ToastProvider } from '@/components/ui/Toast';
import { ErrorBoundary } from '@/components/ui/ErrorBoundary';
import { Sidebar, type NavTab } from '@/components/layout/Sidebar';
import { Header } from '@/components/layout/Header';
import { SetupWizardPage } from '@/pages/Auth/SetupWizardPage';
import { LoginPage } from '@/pages/Auth/LoginPage';
import { api } from '@/lib/api';
import { ApprovalInterruption } from '@/components/features/governance/ApprovalInterruption';
import { ActionProgressToast } from '@/components/features/governance/ActionProgressToast';
import { ActionProgressProvider } from '@/components/providers/ActionProgressProvider';
import { RealtimeProvider } from '@/components/providers/RealtimeProvider';
import { DensityProvider } from '@/components/providers/DensityProvider';
import { ThemeProvider } from '@/components/providers/ThemeProvider';
import { ModelProvider } from '@/components/providers/ModelProvider';
import { SplashScreen } from '@/components/ui/SplashScreen';
import { TopProgressBar } from '@/components/ui/TopProgressBar';
import { CommandPalette } from '@/components/features/search/CommandPalette';
import { MobileBottomNav } from '@/components/layout/MobileBottomNav';
import { PWAInstallBanner } from '@/components/ui/PWAInstallBanner';

const DashboardPage = lazy(() => import('@/pages/Dashboard/DashboardPage').then((m) => ({ default: m.DashboardPage })));
const AgentsPage = lazy(() => import('@/pages/Agents/AgentsPage').then((m) => ({ default: m.AgentsPage })));
const AgentStudioPage = lazy(() => import('@/pages/Agents/AgentStudioPage').then((m) => ({ default: m.AgentStudioPage })));
const ChatPage = lazy(() => import('@/pages/Chat/ChatPage').then((m) => ({ default: m.ChatPage })));
const MissionsPage = lazy(() => import('@/pages/Missions/MissionsPage').then((m) => ({ default: m.MissionsPage })));
const OperationsPage = lazy(() => import('@/pages/Operations/OperationsPage').then((m) => ({ default: m.OperationsPage })));
const AutomationsPage = lazy(() => import('@/pages/Automations/AutomationsPage').then((m) => ({ default: m.AutomationsPage })));
const PluginsPage = lazy(() => import('@/pages/Plugins/PluginsPage').then((m) => ({ default: m.PluginsPage })));
const ChannelsPage = lazy(() => import('@/pages/Channels/ChannelsPage').then((m) => ({ default: m.ChannelsPage })));
const ToolHubPage = lazy(() => import('@/pages/ToolHub/ToolHubPage').then((m) => ({ default: m.ToolHubPage })));
const SkillsPage = lazy(() => import('@/pages/Skills/SkillsPage').then((m) => ({ default: m.SkillsPage })));
const WorkspacePage = lazy(() => import('@/pages/Workspace/WorkspacePage').then((m) => ({ default: m.WorkspacePage })));
const TerminalPage = lazy(() => import('@/pages/Terminal/TerminalPage').then((m) => ({ default: m.TerminalPage })));
const SettingsPage = lazy(() => import('@/pages/Settings/SettingsPage').then((m) => ({ default: m.SettingsPage })));
const AuditLogsPage = lazy(() => import('@/pages/AuditLogs/AuditLogsPage').then((m) => ({ default: m.AuditLogsPage })));
const NotificationsPage = lazy(() => import('@/pages/Notifications/NotificationsPage').then((m) => ({ default: m.NotificationsPage })));
const CostsPage = lazy(() => import('@/pages/Costs/CostsPage').then((m) => ({ default: m.CostsPage })));

export const navTabs: NavTab[] = [
  'dashboard', 'agents', 'agent-studio', 'chat', 'missions', 'operations',
  'automations', 'plugins', 'channels', 'tools', 'skills', 'workspace', 'terminal', 'notifications', 'audit-logs', 'costs', 'settings',
];

export function tabFromLocation(): NavTab {
  const raw = window.location.hash.replace(/^#\/?/, '');
  const [path, query = ''] = raw.split('?');
  const params = new URLSearchParams(query);
  if (path === 'settings' && params.get('view') === 'tokens') return 'costs';
  if (path === 'agents/new' || path.startsWith('agents/')) return 'agent-studio';
  return navTabs.includes(path as NavTab) ? path as NavTab : 'dashboard';
}

export function App() {
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
  const [showSplash, setShowSplash] = useState(true);
  const [isNavigating, setIsNavigating] = useState(false);

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
    const handleHashChange = () => {
      const raw = window.location.hash.replace(/^#\/?/, '');
      const [path, query = ''] = raw.split('?');
      const params = new URLSearchParams(query);
      if (path === 'settings' && params.get('view') === 'tokens') {
        window.location.hash = '/costs';
        return;
      }
      const nextTab = tabFromLocation();
      const route = path;
      if (nextTab === 'agent-studio' && route.startsWith('agents/')) {
        setStudioAgentID(decodeURIComponent(route.slice('agents/'.length)) || 'new');
      }
      setActiveTab(nextTab);
    };
    window.addEventListener('hashchange', handleHashChange);
    return () => window.removeEventListener('hashchange', handleHashChange);
  }, []);

  useEffect(() => {
    const handleShortcut = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setCommandOpen((prev) => !prev);
      }
    };
    window.addEventListener('keydown', handleShortcut);
    return () => window.removeEventListener('keydown', handleShortcut);
  }, []);

  const navigateTab = (tab: NavTab) => {
    setIsNavigating(true);
    const current = window.location.hash.replace(/^#\/?/, '').split('?')[0];
    if (tab === 'costs' && current === 'costs') {
      setActiveTab(tab);
      setTimeout(() => setIsNavigating(false), 240);
      return;
    }
    if (window.location.hash !== `#/${tab}`) window.location.hash = `/${tab}`;
    setActiveTab(tab);
    setTimeout(() => setIsNavigating(false), 240);
  };

  const handleLogout = async () => {
    try {
      await api.logout();
    } catch { }
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

  if (showSplash) {
    return (
      <ThemeProvider>
        <SplashScreen isReady={!authStatus.loading} onComplete={() => setShowSplash(false)} />
      </ThemeProvider>
    );
  }

  if (!authStatus.initialized) {
    return (
      <ThemeProvider>
        <ToastProvider>
          <SetupWizardPage onCompleted={checkAuth} />
        </ToastProvider>
      </ThemeProvider>
    );
  }

  if (!authStatus.authenticated) {
    return (
      <ThemeProvider>
        <ToastProvider>
          <LoginPage userName={authStatus.userName} onAuthenticated={checkAuth} />
        </ToastProvider>
      </ThemeProvider>
    );
  }

  return (
    <ThemeProvider>
      <ToastProvider>
        <DensityProvider>
          <RealtimeProvider>
            <ModelProvider>
              <ActionProgressProvider>
                <div className="min-h-screen bg-canvas text-deep-ink selection:bg-hi-yellow selection:text-deep-ink font-sans flex">
                  <TopProgressBar isLoading={isNavigating} />
                  <PWAInstallBanner />
                  <ApprovalInterruption />
                  <ActionProgressToast />
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
                    className={`flex-1 flex flex-col min-w-0 transition-all duration-200 ease-in-out ${collapsed ? 'lg:ml-20' : 'lg:ml-64'
                      }`}
                  >
                    {/* Sticky Top Header */}
                    <Header
                      activeTab={activeTab}
                      onOpenMobileSidebar={() => setMobileOpen(true)}
                      collapsed={collapsed}
                      onLogout={handleLogout}
                      onOpenSearch={() => setCommandOpen(true)}
                      onNavigateTab={navigateTab}
                      onOpenChat={handleOpenChatWithAgent}
                    />

                    {/* Page Views */}
                    <main className="flex-1 w-full pb-20 lg:pb-12">
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
                          {activeTab === 'plugins' && <PluginsPage />}
                          {activeTab === 'channels' && <ChannelsPage onNavigateTab={navigateTab} />}
                          {activeTab === 'tools' && <ToolHubPage />}
                          {activeTab === 'skills' && <SkillsPage />}
                          {activeTab === 'workspace' && <WorkspacePage />}
                          {activeTab === 'terminal' && <TerminalPage />}
                          {activeTab === 'notifications' && (
                            <NotificationsPage
                              onNavigateTab={navigateTab}
                            />
                          )}
                          {activeTab === 'audit-logs' && <AuditLogsPage />}
                          {activeTab === 'costs' && <CostsPage />}
                          {activeTab === 'settings' && <SettingsPage />}
                        </Suspense>
                      </ErrorBoundary>
                    </main>

                    {/* Mobile Bottom Navigation Bar (< lg screens) */}
                    <MobileBottomNav
                      activeTab={activeTab}
                      onSelectTab={navigateTab}
                      onOpenSidebar={() => setMobileOpen(true)}
                    />
                  </div>
                </div>
              </ActionProgressProvider>
            </ModelProvider>
          </RealtimeProvider>
        </DensityProvider>
      </ToastProvider>
    </ThemeProvider>
  );
}

export default App;
