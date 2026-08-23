import type { TariffBreakdown, TariffRequest, TariffTierLine, Tier } from '../types'

export const OPEN_UPPER_BOUND = 1e12

export const DEFAULT_USD_PER_EGP = 48.5

function round4(value: number): number {
  return Math.round(value * 10000) / 10000
}

function tierLabel(tier: Tier): string {
  if (tier.upper_kwh >= OPEN_UPPER_BOUND) {
    return `${tier.lower_kwh}+ kWh`
  }
  if (tier.lower_kwh === 0) {
    return `0-${tier.upper_kwh} kWh`
  }
  return `${tier.lower_kwh + 1}-${tier.upper_kwh} kWh`
}

function normalizeTiers(tiers: Tier[]): Tier[] {
  const normalized = tiers.map((tier) => ({
    ...tier,
    upper_kwh: tier.upper_kwh >= OPEN_UPPER_BOUND || tier.upper_kwh <= 0 ? Number.MAX_VALUE : tier.upper_kwh,
  }))
  return normalized
}

export function calculateTariff(req: TariffRequest): TariffBreakdown {
  const usdPerEGP = req.usd_per_egp && req.usd_per_egp > 0 ? req.usd_per_egp : DEFAULT_USD_PER_EGP

  if (req.mode === 'flat') {
    const total = round4(req.kwh * (req.flat_rate_egp ?? 0))
    const effective = req.kwh > 0 ? round4(total / req.kwh) : (req.flat_rate_egp ?? 0)
    return {
      mode: 'flat',
      kwh: round4(req.kwh),
      currency: 'EGP',
      total_cost_egp: total,
      total_cost_usd: round4(total / usdPerEGP),
      effective_rate_per_kwh_egp: effective,
      tiers: [
        {
          name: 'Flat Rate',
          label: 'Flat Rate',
          lower_kwh: 0,
          upper_kwh: req.kwh,
          kwh_used: req.kwh,
          rate_egp: req.flat_rate_egp ?? 0,
          cost_egp: total,
          cost_usd: round4(total / usdPerEGP),
        },
      ],
    }
  }

  const tiers = normalizeTiers(req.tiers ?? [])
  let remaining = req.kwh
  const lines: TariffTierLine[] = []
  let total = 0

  for (const tier of tiers) {
    const width = tier.upper_kwh - tier.lower_kwh
    const used = Math.max(0, Math.min(remaining, width))
    const cost = used * tier.rate_egp
    lines.push({
      name: tier.name,
      label: tierLabel(tier),
      lower_kwh: tier.lower_kwh,
      upper_kwh: tier.upper_kwh >= Number.MAX_VALUE ? 0 : tier.upper_kwh,
      kwh_used: round4(used),
      rate_egp: tier.rate_egp,
      cost_egp: round4(cost),
      cost_usd: round4(cost / usdPerEGP),
    })
    total += cost
    remaining -= used
    if (remaining <= 0) {
      remaining = 0
    }
  }

  total = round4(total)
  const effective = req.kwh > 0 ? round4(total / req.kwh) : 0

  return {
    mode: 'tiered',
    kwh: round4(req.kwh),
    currency: 'EGP',
    total_cost_egp: total,
    total_cost_usd: round4(total / usdPerEGP),
    effective_rate_per_kwh_egp: effective,
    tiers: lines,
  }
}
