// Common cron presets grouped by category, offered as a dropdown next to the
// free-form cron input in the schedule modal.
import { i18next } from '@/i18n'

const text = (key: string) => i18next.t(key, { ns: 'schedules' })

export const PRESET_GROUPS: { group: string; items: { label: string; expr: string }[] }[] = [
  {
    get group() {
      return text('cron.groups.minutes')
    },
    items: [
      {
        get label() {
          return text('cron.presets.everyMinute')
        },
        expr: '* * * * *',
      },
      {
        get label() {
          return text('cron.presets.every5Minutes')
        },
        expr: '*/5 * * * *',
      },
      {
        get label() {
          return text('cron.presets.every10Minutes')
        },
        expr: '*/10 * * * *',
      },
      {
        get label() {
          return text('cron.presets.every15Minutes')
        },
        expr: '*/15 * * * *',
      },
      {
        get label() {
          return text('cron.presets.every30Minutes')
        },
        expr: '*/30 * * * *',
      },
    ],
  },
  {
    get group() {
      return text('cron.groups.hours')
    },
    items: [
      {
        get label() {
          return text('cron.presets.hourly')
        },
        expr: '0 * * * *',
      },
      {
        get label() {
          return text('cron.presets.every2Hours')
        },
        expr: '0 */2 * * *',
      },
      {
        get label() {
          return text('cron.presets.every4Hours')
        },
        expr: '0 */4 * * *',
      },
      {
        get label() {
          return text('cron.presets.every6Hours')
        },
        expr: '0 */6 * * *',
      },
      {
        get label() {
          return text('cron.presets.every12Hours')
        },
        expr: '0 */12 * * *',
      },
    ],
  },
  {
    get group() {
      return text('cron.groups.daily')
    },
    items: [
      {
        get label() {
          return text('cron.presets.midnight')
        },
        expr: '0 0 * * *',
      },
      { label: '06:00', expr: '0 6 * * *' },
      { label: '08:00', expr: '0 8 * * *' },
      { label: '09:00', expr: '0 9 * * *' },
      { label: '12:00', expr: '0 12 * * *' },
      { label: '18:00', expr: '0 18 * * *' },
      { label: '21:00', expr: '0 21 * * *' },
    ],
  },
  {
    get group() {
      return text('cron.groups.weekly')
    },
    items: [
      {
        get label() {
          return text('cron.presets.weekdaysAt0900')
        },
        expr: '0 9 * * 1-5',
      },
      {
        get label() {
          return text('cron.presets.mondayAt0800')
        },
        expr: '0 8 * * 1',
      },
      {
        get label() {
          return text('cron.presets.mondayAt0900')
        },
        expr: '0 9 * * 1',
      },
      {
        get label() {
          return text('cron.presets.fridayAt1700')
        },
        expr: '0 17 * * 5',
      },
      {
        get label() {
          return text('cron.presets.weekendsAt1000')
        },
        expr: '0 10 * * 6,0',
      },
    ],
  },
  {
    get group() {
      return text('cron.groups.monthly')
    },
    items: [
      {
        get label() {
          return text('cron.presets.firstAt0900')
        },
        expr: '0 9 1 * *',
      },
      {
        get label() {
          return text('cron.presets.fifteenthAt0900')
        },
        expr: '0 9 15 * *',
      },
      {
        get label() {
          return text('cron.presets.monthEndAt1800')
        },
        expr: '0 18 28-31 * *',
      },
    ],
  },
]
