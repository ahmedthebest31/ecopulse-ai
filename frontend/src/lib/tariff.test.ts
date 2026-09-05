import { describe, expect, it } from 'vitest'
import { DEFAULT_USD_PER_EGP, OPEN_UPPER_BOUND, calculateTariff } from './tariff'
import type { TariffRequest, Tier } from '../types'

const PRD_TIERS: Tier[] = [
  { name: 'T1', lower_kwh: 0, upper_kwh: 50, rate_egp: 0.68 },
  { name: 'T2', lower_kwh: 51, upper_kwh: 100, rate_egp: 0.78 },
  { name: 'T3', lower_kwh: 101, upper_kwh: 200, rate_egp: 0.95 },
  { name: 'T4', lower_kwh: 201, upper_kwh: 350, rate_egp: 1.55 },
  { name: 'T5', lower_kwh: 351, upper_kwh: 650, rate_egp: 1.95 },
  { name: 'T6', lower_kwh: 651, upper_kwh: 1000, rate_egp: 2.1 },
  { name: 'T7', lower_kwh: 1001, upper_kwh: OPEN_UPPER_BOUND, rate_egp: 2.23 },
]

function flatRequest(overrides: Partial<TariffRequest> = {}): TariffRequest {
  return { kwh: 100, mode: 'flat', flat_rate_egp: 2.5, ...overrides }
}

function tieredRequest(overrides: Partial<TariffRequest> = {}): TariffRequest {
  return { kwh: 300, mode: 'tiered', tiers: PRD_TIERS, ...overrides }
}

describe('calculateTariff flat mode', () => {
  it('bills kwh * flat_price with default USD rate', () => {
    const result = calculateTariff(flatRequest())
    expect(result.mode).toBe('flat')
    expect(result.currency).toBe('EGP')
    expect(result.kwh).toBe(100)
    expect(result.total_cost_egp).toBe(250)
    expect(result.total_cost_usd).toBe(5.1546)
    expect(result.effective_rate_per_kwh_egp).toBe(2.5)
    expect(result.tiers).toHaveLength(1)
    expect(result.tiers[0]).toMatchObject({
      name: 'Flat Rate',
      label: 'Flat Rate',
      lower_kwh: 0,
      upper_kwh: 100,
      kwh_used: 100,
      rate_egp: 2.5,
      cost_egp: 250,
      cost_usd: 5.1546,
    })
  })

  it('uses DEFAULT_USD_PER_EGP when usd_per_egp is omitted', () => {
    const result = calculateTariff(flatRequest())
    expect(result.total_cost_usd).toBe(5.1546)
    expect(DEFAULT_USD_PER_EGP).toBe(48.5)
  })

  it('honours a custom positive usd_per_egp', () => {
    const result = calculateTariff(flatRequest({ usd_per_egp: 50 }))
    expect(result.total_cost_usd).toBe(5)
  })

  it('falls back to default USD rate for usd_per_egp 0 and negative', () => {
    expect(calculateTariff(flatRequest({ usd_per_egp: 0 })).total_cost_usd).toBe(5.1546)
    expect(calculateTariff(flatRequest({ usd_per_egp: -1 })).total_cost_usd).toBe(5.1546)
  })

  it('zero kwh keeps flat rate as effective rate', () => {
    const result = calculateTariff(flatRequest({ kwh: 0 }))
    expect(result.total_cost_egp).toBe(0)
    expect(result.effective_rate_per_kwh_egp).toBe(2.5)
  })

  it('missing flat_rate defaults to zero', () => {
    const result = calculateTariff(flatRequest({ flat_rate_egp: undefined }))
    expect(result.total_cost_egp).toBe(0)
    expect(result.effective_rate_per_kwh_egp).toBe(0)
  })

  it('rounds every money value to 4 decimal places', () => {
    const result = calculateTariff(flatRequest({ kwh: 1 / 3, flat_rate_egp: 1 }))
    expect(result.kwh).toBe(0.3333)
    expect(result.total_cost_egp).toBe(0.3333)
    expect(result.total_cost_usd).toBe(0.0069)
    expect(result.effective_rate_per_kwh_egp).toBe(0.9999)
  })
})

describe('calculateTariff tiered mode', () => {
  it('allocates 300 kwh incrementally across PRD tiers', () => {
    const result = calculateTariff(tieredRequest())
    expect(result.mode).toBe('tiered')
    expect(result.total_cost_egp).toBe(324.37)
    expect(result.total_cost_usd).toBe(6.688)
    expect(result.effective_rate_per_kwh_egp).toBe(1.0812)
    expect(result.tiers).toHaveLength(7)
    expect(result.tiers.map((t) => t.kwh_used)).toEqual([50, 49, 99, 102, 0, 0, 0])
    expect(result.tiers.map((t) => t.cost_egp)).toEqual([34, 38.22, 94.05, 158.1, 0, 0, 0])
  })

  it('produces correct labels and open-tier upper_kwh 0', () => {
    const result = calculateTariff(tieredRequest())
    expect(result.tiers.map((t) => t.label)).toEqual([
      '0-50 kWh',
      '52-100 kWh',
      '102-200 kWh',
      '202-350 kWh',
      '352-650 kWh',
      '652-1000 kWh',
      '1001+ kWh',
    ])
    expect(result.tiers[0].upper_kwh).toBe(50)
    expect(result.tiers[6].upper_kwh).toBe(0)
  })

  it('handles consumption beyond the final open tier', () => {
    const result = calculateTariff(tieredRequest({ kwh: 2000 }))
    expect(result.total_cost_egp).toBe(3954.32)
    expect(result.tiers[6].kwh_used).toBe(1005)
    expect(result.tiers[6].cost_egp).toBe(2241.15)
  })

  it('stops allocating after kwh is exhausted', () => {
    const result = calculateTariff(tieredRequest({ kwh: 100 }))
    expect(result.total_cost_egp).toBe(73.17)
    expect(result.tiers.map((t) => t.kwh_used)).toEqual([50, 49, 1, 0, 0, 0, 0])
  })

  it('consumes exactly the top of a tier at its boundary', () => {
    const result = calculateTariff(tieredRequest({ kwh: 198 }))
    expect(result.total_cost_egp).toBe(166.27)
    expect(result.tiers[2].kwh_used).toBe(99)
  })

  it('zero kwh yields no consumption and zero cost', () => {
    const result = calculateTariff(tieredRequest({ kwh: 0 }))
    expect(result.total_cost_egp).toBe(0)
    expect(result.effective_rate_per_kwh_egp).toBe(0)
    expect(result.tiers.every((t) => t.kwh_used === 0)).toBe(true)
  })

  it('missing tiers yields an empty breakdown with zero cost', () => {
    const result = calculateTariff(tieredRequest({ tiers: undefined }))
    expect(result.tiers).toEqual([])
    expect(result.total_cost_egp).toBe(0)
  })

  it('normalizes an upper bound of 0 into an open tier', () => {
    const result = calculateTariff({
      kwh: 3,
      mode: 'tiered',
      tiers: [{ name: 'Open', lower_kwh: 0, upper_kwh: 0, rate_egp: 2 }],
    })
    expect(result.tiers).toHaveLength(1)
    expect(result.tiers[0].label).toBe('0+ kWh')
    expect(result.tiers[0].upper_kwh).toBe(0)
    expect(result.tiers[0].kwh_used).toBe(3)
    expect(result.total_cost_egp).toBe(6)
  })

  it('a normalizing open tier uses the whole opening as one bracket', () => {
    const result = calculateTariff({
      kwh: 5,
      mode: 'tiered',
      tiers: [{ name: 'Open', lower_kwh: 0, upper_kwh: OPEN_UPPER_BOUND, rate_egp: 1 }],
    })
    expect(result.tiers[0].label).toBe('0+ kWh')
    expect(result.total_cost_egp).toBe(5)
  })

  it('allocates by tier width ignoring gaps between tiers', () => {
    const result = calculateTariff({
      kwh: 300,
      mode: 'tiered',
      tiers: [
        { name: 'A', lower_kwh: 0, upper_kwh: 50, rate_egp: 1 },
        { name: 'B', lower_kwh: 200, upper_kwh: 350, rate_egp: 2 },
      ],
    })
    expect(result.total_cost_egp).toBe(350)
    expect(result.tiers.map((t) => t.kwh_used)).toEqual([50, 150])
  })

  it('a zero-width non-open tier contributes nothing', () => {
    const result = calculateTariff({
      kwh: 5,
      mode: 'tiered',
      tiers: [
        { name: 'Wide', lower_kwh: 0, upper_kwh: 100, rate_egp: 1 },
        { name: 'None', lower_kwh: 50, upper_kwh: 50, rate_egp: 999 },
      ],
    })
    expect(result.total_cost_egp).toBe(5)
    expect(result.tiers[1].kwh_used).toBe(0)
  })

  it('honours a custom usd_per_egp in tiered mode', () => {
    const result = calculateTariff(tieredRequest({ usd_per_egp: 100 }))
    expect(result.total_cost_usd).toBe(3.2437)
  })

  it('uses DEFAULT_USD_PER_EGP when usd_per_egp is 0 in tiered mode', () => {
    const result = calculateTariff(tieredRequest({ usd_per_egp: 0 }))
    expect(result.total_cost_usd).toBe(6.688)
  })

  it('returns back a negative kwh with zero cost', () => {
    const result = calculateTariff(tieredRequest({ kwh: -5 }))
    expect(result.kwh).toBe(-5)
    expect(result.total_cost_egp).toBe(0)
    expect(result.effective_rate_per_kwh_egp).toBe(0)
  })
})