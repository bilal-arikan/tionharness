import { afterEach, describe, expect, it } from 'vitest'
import { i18next } from '@/i18n'
import { NODE_TYPES } from './flowsPanelShared'
import { chromeFor } from './nodeStyles'
import { nodeTypeHelp } from './nodeTypeHelp'
import { statusLabel } from './runStatus'
import {
  FLOW_TEMPLATES,
  flowTemplateCompanions,
  flowTemplateDescription,
  flowTemplateGraph,
  flowTemplateName,
} from './flowTemplates'
import { blankNode, ensureStartNode } from './flowGraph'

afterEach(async () => {
  await i18next.changeLanguage('tr')
})

describe('flows localization', () => {
  it('resolves module labels from the active locale instead of freezing them at import time', async () => {
    const startType = NODE_TYPES.find((type) => type.value === 'start')!

    await i18next.changeLanguage('en')
    expect(startType.label).toBe('Start')
    expect(chromeFor('start').label).toBe('Start')
    expect(statusLabel('success')).toBe('✓ succeeded')

    await i18next.changeLanguage('tr')
    expect(startType.label).toBe('Başlangıç')
    expect(chromeFor('start').label).toBe('Başlangıç')
    expect(statusLabel('success')).toBe('✓ başarılı')
  })

  it('localizes template display metadata without changing the stored template graph', async () => {
    const template = FLOW_TEMPLATES.find((item) => item.id === 'default-starter')!
    const storedName = template.name
    const storedGraph = template.graph

    await i18next.changeLanguage('en')
    expect(flowTemplateName(template)).toBe('Answer & Verify')
    expect(flowTemplateDescription(template)).toContain('default flow')

    await i18next.changeLanguage('tr')
    expect(flowTemplateName(template)).toBe('Yanıtla & Doğrula')
    expect(template.name).toBe(storedName)
    expect(template.graph).toBe(storedGraph)
  })

  it('localizes template preview and creation titles without changing template payloads', async () => {
    const template = FLOW_TEMPLATES.find((item) => item.id === 'async-fanout')!
    const storedGraph = template.graph
    const storedStart = storedGraph.nodes.find((node) => node.id === 'start')!
    const storedSpawn = storedGraph.nodes.find((node) => node.id === 'spawn')!
    const storedSynth = storedGraph.nodes.find((node) => node.id === 'synth')!

    await i18next.changeLanguage('en')
    const englishGraph = flowTemplateGraph(template)
    expect(englishGraph.nodes.find((node) => node.id === 'start')?.title).toBe('Start')
    expect(englishGraph.nodes.find((node) => node.id === 'spawn')?.title).toBe('Start Analyses')
    expect(englishGraph.nodes.find((node) => node.id === 'synth')).toEqual({
      ...storedSynth,
      title: 'Synthesize',
    })

    await i18next.changeLanguage('tr')
    const turkishGraph = flowTemplateGraph(template)
    expect(turkishGraph.nodes.find((node) => node.id === 'start')?.title).toBe('Başlangıç')
    expect(turkishGraph.nodes.find((node) => node.id === 'spawn')?.title).toBe('Analizleri Başlat')
    expect(template.graph).toBe(storedGraph)
    expect(storedStart.title).toBe('Başlangıç')
    expect(storedSpawn.title).toBe('Analizleri Başlat')
    expect(storedSynth.title).toBe('Birleştir')
  })

  it('localizes companion flow names and node titles without changing their stored graphs', async () => {
    const template = FLOW_TEMPLATES.find((item) => item.id === 'async-fanout')!
    const storedCompanion = template.companions![0]
    const storedTransform = storedCompanion.graph.nodes.find((node) => node.id === 't')!

    await i18next.changeLanguage('en')
    const englishCompanion = flowTemplateCompanions(template)![0]
    expect(englishCompanion.name).toBe('Analysis — Positive Perspective')
    expect(englishCompanion.graph.nodes.find((node) => node.id === 'start')?.title).toBe('Start')
    expect(englishCompanion.graph.nodes.find((node) => node.id === 't')).toEqual({
      ...storedTransform,
      title: 'Positive',
    })

    await i18next.changeLanguage('tr')
    const turkishCompanion = flowTemplateCompanions(template)![0]
    expect(turkishCompanion.name).toBe('Analiz — Olumlu Bakış')
    expect(turkishCompanion.graph.nodes.find((node) => node.id === 't')?.title).toBe('Olumlu')
    expect(template.companions![0]).toBe(storedCompanion)
    expect(storedTransform.title).toBe('Olumlu')
  })

  it('has display-title catalog entries for every built-in template node', () => {
    for (const language of ['en', 'tr']) {
      for (const template of FLOW_TEMPLATES) {
        for (const node of template.graph.nodes) {
          if (node.type === 'start' || node.type === 'end') continue
          expect(
            i18next.getResource(language, 'flows', `templates.${template.id}.nodes.${node.id}`),
          ).toEqual(expect.any(String))
        }

        for (const [index, companion] of (template.companions ?? []).entries()) {
          expect(
            i18next.getResource(
              language,
              'flows',
              `templates.${template.id}.companions.${index}.name`,
            ),
          ).toEqual(expect.any(String))
          for (const node of companion.graph.nodes) {
            if (node.type === 'start' || node.type === 'end') continue
            expect(
              i18next.getResource(
                language,
                'flows',
                `templates.${template.id}.companions.${index}.nodes.${node.id}`,
              ),
            ).toEqual(expect.any(String))
          }
        }
      }
    }
  })

  it('localizes node help dynamically', async () => {
    await i18next.changeLanguage('en')
    expect(nodeTypeHelp('join')).toContain('Barrier node')

    await i18next.changeLanguage('tr')
    expect(nodeTypeHelp('join')).toContain('Bariyer düğümü')
  })

  it('uses the active locale for newly generated node titles', async () => {
    await i18next.changeLanguage('en')
    expect(blankNode('start-en', 'start').title).toBe('Start')
    expect(blankNode('input-en', 'await-input').title).toBe('Wait for Input')
    expect(ensureStartNode({ start: 'agent', nodes: [] }).nodes[0].title).toBe('Start')

    await i18next.changeLanguage('tr')
    expect(blankNode('start-tr', 'start').title).toBe('Başlangıç')
    expect(blankNode('input-tr', 'await-input').title).toBe('Girdi Bekle')
    expect(ensureStartNode({ start: 'agent', nodes: [] }).nodes[0].title).toBe('Başlangıç')
  })

  it('does not rewrite an existing saved node title', async () => {
    const graph = {
      start: 'custom-start',
      nodes: [
        {
          id: 'custom-start',
          type: 'start' as const,
          title: 'My custom entry',
          next: '',
        },
      ],
    }

    await i18next.changeLanguage('tr')
    expect(ensureStartNode(graph)).toBe(graph)
    expect(graph.nodes[0].title).toBe('My custom entry')
  })
})
