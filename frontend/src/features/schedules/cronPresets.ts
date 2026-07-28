// Common cron presets grouped by category, offered as a dropdown next to the
// free-form cron input in the schedule modal.
export const PRESET_GROUPS: { group: string; items: { label: string; expr: string }[] }[] = [
  {
    group: 'Dakikalar',
    items: [
      { label: 'Her dakika',    expr: '* * * * *'    },
      { label: 'Her 5 dakika',  expr: '*/5 * * * *'  },
      { label: 'Her 10 dakika', expr: '*/10 * * * *' },
      { label: 'Her 15 dakika', expr: '*/15 * * * *' },
      { label: 'Her 30 dakika', expr: '*/30 * * * *' },
    ],
  },
  {
    group: 'Saatler',
    items: [
      { label: 'Saat başı',    expr: '0 * * * *'    },
      { label: 'Her 2 saatte', expr: '0 */2 * * *'  },
      { label: 'Her 4 saatte', expr: '0 */4 * * *'  },
      { label: 'Her 6 saatte', expr: '0 */6 * * *'  },
      { label: 'Her 12 saatte',expr: '0 */12 * * *' },
    ],
  },
  {
    group: 'Günlük',
    items: [
      { label: 'Gece yarısı', expr: '0 0 * * *'  },
      { label: '06:00',       expr: '0 6 * * *'  },
      { label: '08:00',       expr: '0 8 * * *'  },
      { label: '09:00',       expr: '0 9 * * *'  },
      { label: '12:00',       expr: '0 12 * * *' },
      { label: '18:00',       expr: '0 18 * * *' },
      { label: '21:00',       expr: '0 21 * * *' },
    ],
  },
  {
    group: 'Haftalık',
    items: [
      { label: 'Hafta içi 09:00',  expr: '0 9 * * 1-5'  },
      { label: 'Pazartesi 08:00',  expr: '0 8 * * 1'    },
      { label: 'Pazartesi 09:00',  expr: '0 9 * * 1'    },
      { label: 'Cuma 17:00',       expr: '0 17 * * 5'   },
      { label: 'Hafta sonu 10:00', expr: '0 10 * * 6,0' },
    ],
  },
  {
    group: 'Aylık',
    items: [
      { label: "Ayın 1'i 09:00",  expr: '0 9 1 * *'  },
      { label: "Ayın 15'i 09:00", expr: '0 9 15 * *' },
      { label: "Ayın sonu 18:00", expr: '0 18 28-31 * *' },
    ],
  },
]
