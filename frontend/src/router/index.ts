import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'fleet-overview',
      component: () => import('../views/DashboardView.vue'),
    },
    {
      path: '/agents',
      name: 'agents',
      component: () => import('../views/AgentsView.vue'),
    },
    {
      path: '/agents/:id',
      name: 'agent-inspect',
      component: () => import('../views/AgentDetailView.vue'),
    },
    {
      path: '/agents/:id/plans/:planId',
      name: 'plan-history',
      component: () => import('../views/PlanHistoryView.vue'),
    },
    {
      path: '/repositories',
      name: 'repositories',
      component: () => import('../views/RepositoriesView.vue'),
    },
    {
      path: '/repositories/new',
      name: 'repository-new',
      component: () => import('../views/RepositoryFormView.vue'),
    },
    {
      path: '/repositories/:id/edit',
      name: 'repository-edit',
      component: () => import('../views/RepositoryFormView.vue'),
    },
    {
      path: '/plans',
      name: 'plans',
      component: () => import('../views/PlansView.vue'),
    },
    {
      path: '/plans/new',
      name: 'plan-new',
      component: () => import('../views/PlanFormView.vue'),
    },
    {
      path: '/plans/:id',
      name: 'plan-detail',
      component: () => import('../views/PlanDetailView.vue'),
    },
    {
      path: '/plans/:id/edit',
      name: 'plan-edit',
      component: () => import('../views/PlanFormView.vue'),
    },
    {
      path: '/scripts',
      name: 'scripts',
      component: () => import('../views/ScriptsView.vue'),
    },
    {
      path: '/scripts/new',
      name: 'script-new',
      component: () => import('../views/ScriptFormView.vue'),
    },
    {
      path: '/scripts/:id/edit',
      name: 'script-edit',
      component: () => import('../views/ScriptFormView.vue'),
    },
    {
      path: '/jobs',
      name: 'jobs',
      component: () => import('../views/JobsView.vue'),
    },
    {
      path: '/jobs/:id',
      name: 'job-detail',
      component: () => import('../views/JobDetailView.vue'),
    },
    {
      path: '/agents/:id/plans/:planId/jobs/:jobId',
      name: 'job-console',
      component: () => import('../views/JobDetailView.vue'),
    },
    {
      path: '/snapshots',
      name: 'snapshots',
      component: () => import('../views/SnapshotsView.vue'),
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('../views/SettingsView.vue'),
    },
  ],
})

export default router
