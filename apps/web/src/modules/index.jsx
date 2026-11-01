import salesModule from './sales';

// Built-in frontend modules are composed here. A future module contributes
// through this descriptor without adding branches to core pages.
export const installedModules = Object.freeze([salesModule]);

export const routeContributions = (modules = installedModules) => modules.flatMap((module) => module.routes.map((route) => ({ ...route, moduleId: module.id })));
export const navigationContributions = (features, t, modules = installedModules) => modules.filter((module) => module.enabled(features)).map((module) => ({ id: module.id, ...module.navigation(t) }));
export const dashboardContributions = (features, modules = installedModules) => modules.filter((module) => module.enabled(features) && module.Dashboard).map((module) => ({ id: module.id, Component: module.Dashboard }));
export const printJobCardContributions = (features, modules = installedModules) => modules.filter((module) => module.enabled(features) && module.PrintJobCardDetail).map((module) => ({ id: module.id, Component: module.PrintJobCardDetail }));
export const featureSettingContributions = (modules = installedModules) => modules.filter((module) => module.FeatureSetting).map((module) => ({ id: module.id, Component: module.FeatureSetting }));
