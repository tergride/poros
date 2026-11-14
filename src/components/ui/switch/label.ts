import type { InjectionKey } from 'vue'

/** The id of the title of the row a switch sits in: a switch without a name of its own is named by it. */
export const rowLabel: InjectionKey<string> = Symbol('rowLabel')
