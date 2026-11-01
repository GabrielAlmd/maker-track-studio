// Bundled, versioned examples. Values are copied once and never linked to a profile.
const profile = (purchase, upfront, maintenance, lifetime, uptime, watts, buffer) => ({
  machine_cost_mode: 1, printer_purchase_cost: purchase, printer_upfront_cost: upfront,
  printer_annual_maintenance: maintenance, printer_lifetime_years: lifetime,
  printer_uptime_percent: uptime, printer_power_watts: watts, printer_buffer_factor: buffer,
});

const rows = [
  ['bambu-lab-a1-v1','Bambu Lab','A1','Bambu Lab A1',299,35,30,3,50,150,1.3,1300,'User reference profile','Perfil de referência fornecido pelo utilizador.','https://cdn1.bambulab.com/documentation/quick-start-a75adcb1d5d5e/Quick%20Start%20Guide%20for%20A1.pdf','high'],
  ['bambu-lab-a1-mini-v1','Bambu Lab','A1 mini','Bambu Lab A1 mini',199,25,25,3,50,80,1.3,150,'Provisional average estimate; validate with a plug meter.','Estimativa provisória da média; validar com um medidor de tomada.','https://bambulab.cn/zh-cn/a1-mini/tech-specs','medium'],
  ['bambu-lab-p1s-v1','Bambu Lab','P1S','Bambu Lab P1S',599,50,40,3,50,180,1.3,1000,'Provisional average estimate; published maximum is not average draw.','Estimativa provisória da média; a potência máxima publicada não representa o consumo médio.','https://eu.store.bambulab.com/en-lt/products/p1s','medium-low'],
  ['bambu-lab-x1-carbon-v1','Bambu Lab','X1 Carbon','Bambu Lab X1 Carbon',1199,75,60,4,50,200,1.3,1000,'Provisional average estimate; published maximum is not average draw.','Estimativa provisória da média; a potência máxima publicada não representa o consumo médio.','https://public-cdn.bambulab.com/store/X1-Carbon%20tech%20specs.pdf','medium-low'],
  ['prusa-mk4s-v1','Prusa','MK4S','Original Prusa MK4S',899,60,50,5,45,90,1.25,240,'Provisional PT/EU price example; mixed-material power estimate.','Preço de exemplo provisório PT/UE; estimativa de potência para materiais mistos.','https://www.prusa3d.com/product/original-prusa-mk4s-kit/','high'],
  ['prusa-core-one-plus-v1','Prusa','CORE One+','Prusa CORE One+',999,60,60,5,45,100,1.25,200,'Provisional PT/EU price example; midpoint power estimate.','Preço de exemplo provisório PT/UE; estimativa média de potência.','https://www.prusa3d.com/core-one/','high'],
  ['creality-ender-3-v3-se-v1','Creality','Ender-3 V3 SE','Creality Ender-3 V3 SE',199,35,25,3,40,120,1.35,350,'Provisional estimate; validate measured draw and parts history.','Estimativa provisória; validar consumo medido e histórico de peças.','https://devstore.creality.com/blogs/all/ender-series-3d-printers-parameter-comparison','medium-low'],
  ['creality-k1c-v1','Creality','K1C','Creality K1C',499,50,40,3,45,150,1.3,350,'Provisional estimate; validate measured draw and maintenance.','Estimativa provisória; validar consumo medido e manutenção.','https://www.creality.com/products/k1c-carbon-3d-printer','medium-low'],
  ['anycubic-kobra-3-v1','Anycubic','Kobra 3','Anycubic Kobra 3',199,40,30,3,45,150,1.3,400,'Provisional base-printer estimate; excludes ACE Pro.','Estimativa provisória apenas para a impressora base; exclui o ACE Pro.','https://store.anycubic.com/products/anycubic-kobra-3','medium-low'],
  ['elegoo-neptune-4-pro-v1','ELEGOO','Neptune 4 Pro','ELEGOO Neptune 4 Pro',229,35,30,3,45,150,1.3,250,'Provisional estimate; validate small and full-bed print draw.','Estimativa provisória; validar consumo em impressão pequena e de mesa completa.','https://us.elegoo.com/products/elegoo-neptune-4-pro-fdm-3d-printer','medium-low'],
];

export const PRINTER_PRESETS = rows.map(([id, manufacturer, model, displayName, purchase, upfront, maintenance, lifetime, uptime, watts, buffer, ratedPowerWatts, assumptions, assumptionsPT, sourceUrl, confidence]) => ({
  id, schemaVersion: 1, migrationStrategy: 'schemaVersion', manufacturer, model, displayName, reviewedAt: '2026-10-27', currency: 'EUR', region: 'PT/EU',
  scope: 'hardware-only', profileDefaults: profile(purchase, upfront, maintenance, lifetime, uptime, watts, buffer),
  evidence: { sourceUrl, ratedPowerWatts, ratedPowerQualifier: 'manufacturer rated or maximum input; not average print draw', averagePowerEvidence: { en: assumptions, 'pt-PT': assumptionsPT }, confidence },
}));

export const A1_REFERENCE_PROFILE = {
  ...PRINTER_PRESETS[0].profileDefaults,
  printer_wear_cost_hour: 0.08, electricity_cost_kwh: 0.20,
  filament_waste_factor: 1.1, labor_cost_hour: 25,
  margin_tier_1: 80, margin_tier_2: 90, margin_tier_3: 99,
};
