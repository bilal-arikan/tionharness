// Built-in, agent-agnostic flow templates shown in the read-only template
// gallery. Agent nodes carry an empty agentId on purpose — the user assigns
// agents after instantiating the template into a real flow. Positions are
// provided so the read-only preview lays out cleanly without auto-layout.
import type { FlowGraph } from '../types'

export interface FlowTemplate {
  id: string
  name: string
  description: string
  graph: FlowGraph
}

export const FLOW_TEMPLATES: FlowTemplate[] = [
  {
    id: 'pipeline',
    name: 'Sıralı Hat',
    description: 'Araştır → taslak yaz → cilala. Tek bir konudan parlatılmış kısa metin üretir.',
    graph: {
      start: 'research',
      edgeStyle: 'smoothstep',
      nodes: [
        { id: 'research', type: 'agent', title: 'Araştır', agentId: '', prompt: 'Research the topic and list key facts and angles: {{input}}', next: 'draft', x: 100, y: 40 },
        { id: 'draft', type: 'agent', title: 'Taslak Yaz', agentId: '', prompt: 'Write a first draft using these notes:\n{{last}}', next: 'polish', x: 100, y: 200 },
        { id: 'polish', type: 'agent', title: 'Cilala', agentId: '', prompt: 'Polish this draft into a tight final version:\n{{last}}', next: '', x: 100, y: 360 },
      ],
    },
  },
  {
    id: 'branch-router',
    name: 'Duygu Yönlendirici',
    description: 'Dallanma: önce duygu analizi, çıktıya göre pozitifse teşekkür, negatifse özür yanıtı.',
    graph: {
      start: 'classify',
      nodes: [
        { id: 'classify', type: 'agent', title: 'Sınıflandır', agentId: '', prompt: 'Classify the sentiment. Reply with EXACTLY one word: POSITIVE or NEGATIVE.\n{{input}}', next: 'route', x: 100, y: 40 },
        { id: 'route', type: 'branch', title: 'Yönlendir', branches: [{ contains: 'positive', next: 'thanks' }, { contains: '', next: 'apologize' }], x: 100, y: 200 },
        { id: 'thanks', type: 'agent', title: 'Teşekkür Et', agentId: '', prompt: 'Write a warm thank-you reply to this positive feedback: {{input}}', next: '', x: 360, y: 130 },
        { id: 'apologize', type: 'agent', title: 'Özür Dile', agentId: '', prompt: 'Write an empathetic apology reply to this negative feedback: {{input}}', next: '', x: 360, y: 300 },
      ],
    },
  },
  {
    id: 'pros-cons',
    name: 'Artı-Eksi Analizi',
    description: 'Paralel fan-out + join: iki ajan aynı anda lehte/aleyhte yazar, üçüncü dengeli sonuç verir.',
    graph: {
      start: 'debate',
      animated: true,
      nodes: [
        { id: 'debate', type: 'parallel', title: 'Tartışma', parallel: ['pros', 'cons'], joinNext: 'conclude', x: 100, y: 40 },
        { id: 'pros', type: 'agent', title: 'Lehte', agentId: '', prompt: 'List the 3 strongest arguments IN FAVOR of: {{input}}', next: '', x: 360, y: 40 },
        { id: 'cons', type: 'agent', title: 'Aleyhte', agentId: '', prompt: 'List the 3 strongest arguments AGAINST: {{input}}', next: '', x: 360, y: 200 },
        { id: 'conclude', type: 'agent', title: 'Sonuç', agentId: '', prompt: 'Given these pros and cons, write a balanced, decisive conclusion:\n{{last}}', next: '', x: 100, y: 280 },
      ],
    },
  },
  {
    id: 'critique-loop',
    name: 'Eleştir-Düzelt',
    description: 'Taslak → eleştir → düzelt. Bir ajan üretir, ikincisi kusurları bulur, üçüncüsü düzeltir.',
    graph: {
      start: 'draft',
      nodes: [
        { id: 'draft', type: 'agent', title: 'Taslak', agentId: '', prompt: 'Produce a first attempt at: {{input}}', next: 'critique', x: 100, y: 40 },
        { id: 'critique', type: 'agent', title: 'Eleştir', agentId: '', prompt: 'Critique this attempt — list concrete flaws and improvements:\n{{last}}', next: 'revise', x: 100, y: 200 },
        { id: 'revise', type: 'agent', title: 'Düzelt', agentId: '', prompt: 'Rewrite the attempt addressing this critique:\n{{node.critique}}\n\nOriginal:\n{{node.draft}}', next: '', x: 100, y: 360 },
      ],
    },
  },
  {
    id: 'planner-executor',
    name: 'Planla-Uygula',
    description: 'Planlayıcı görevi adımlara böler, uygulayıcı uygular, özetleyici sonucu raporlar.',
    graph: {
      start: 'plan',
      edgeStyle: 'smoothstep',
      nodes: [
        { id: 'plan', type: 'agent', title: 'Planla', agentId: '', prompt: 'Break this goal into a concrete, ordered step list: {{input}}', next: 'execute', x: 100, y: 40 },
        { id: 'execute', type: 'agent', title: 'Uygula', agentId: '', prompt: 'Carry out these steps and report results:\n{{last}}', next: 'summarize', x: 100, y: 200 },
        { id: 'summarize', type: 'agent', title: 'Özetle', agentId: '', prompt: 'Summarize the outcome for the user:\n{{last}}', next: '', x: 100, y: 360 },
      ],
    },
  },
  {
    id: 'experts-synthesis',
    name: 'Çoklu Uzman + Sentez',
    description: 'Üç uzman aynı soruyu farklı açılardan paralel yanıtlar, bir ajan görüşleri sentezler.',
    graph: {
      start: 'panel',
      animated: true,
      nodes: [
        { id: 'panel', type: 'parallel', title: 'Uzman Paneli', parallel: ['e1', 'e2', 'e3'], joinNext: 'synth', x: 100, y: 40 },
        { id: 'e1', type: 'agent', title: 'Teknik', agentId: '', prompt: 'Answer from a technical/engineering angle: {{input}}', next: '', x: 360, y: 20 },
        { id: 'e2', type: 'agent', title: 'Ticari', agentId: '', prompt: 'Answer from a business/commercial angle: {{input}}', next: '', x: 360, y: 160 },
        { id: 'e3', type: 'agent', title: 'Kullanıcı', agentId: '', prompt: 'Answer from an end-user/UX angle: {{input}}', next: '', x: 360, y: 300 },
        { id: 'synth', type: 'agent', title: 'Sentez', agentId: '', prompt: 'Synthesize these expert views into one balanced recommendation:\n{{last}}', next: '', x: 100, y: 320 },
      ],
    },
  },
]
