export type ResourceOverview = {
  sites_total: number;
  sites_active: number;
  databases_total: number;
  databases_active: number;
};

export type ResourceOverviewEnvelope = {
  data: ResourceOverview;
};
