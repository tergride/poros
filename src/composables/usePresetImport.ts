import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { reasonOf, useFailure } from '@/composables/useReason'
import type { Preset, PresetPreview } from '@/platform/core-types'
import { MethodPresetsImport, MethodPresetsRead, MethodQRRead, ReasonInvalidLink } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

const presetPrefix = 'poros://route/'

/** A preset about to be imported: what the core read of it and the link or the text of the file it read. */
export interface IncomingPreset extends PresetPreview {
  link: string
}

/** The pictures the core reads QR codes from. */
export const pictureTypes = ['image/png', 'image/jpeg', 'image/gif']

/** What the file picker of an import offers: a picture of a QR code or a preset file. */
export const importAccept = [...pictureTypes, '.json', 'application/json'].join(',')

// The core takes a picture of up to 32 MB and a preset file of up to 1 MB.
const maxPicture = 32 << 20
const maxPresetFile = 1 << 20

/** The first link to a preset among texts, as a QR code holds them. */
export function presetLinkIn(texts: string[]): string | undefined {
  return texts.map((text) => text.trim()).find((text) => text.toLowerCase().startsWith(presetPrefix))
}

const isPresetFile = (file: File) => file.type === 'application/json' || file.name.toLowerCase().endsWith('.json')

/** The file to import among dropped or pasted ones: a picture or a preset file. */
export function importableIn(files: Iterable<File> | undefined | null): File | undefined {
  return [...(files ?? [])].find((file) => pictureTypes.includes(file.type) || isPresetFile(file))
}

async function base64Of(blob: Blob): Promise<string> {
  const bytes = new Uint8Array(await blob.arrayBuffer())
  let binary = ''
  for (let i = 0; i < bytes.length; i += 0x8000) binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  return btoa(binary)
}

/**
 * Adds a preset from a poros://route/ link, a picture of its QR code or its file. The core reads the picture and the
 * file; what goes wrong is told in error, in the words of what the user gave. A name of the user's preset adds
 * nothing: it waits in taken for the question of SameNamePresetDialog.
 */
export function usePresetImport() {
  const core = useCoreStore()
  const failure = useFailure()
  const { t } = useI18n()
  const busy = ref(false)
  const error = ref('')
  const taken = ref<IncomingPreset | null>(null)

  async function linkOnPicture(picture: Blob): Promise<string> {
    if (picture.size > maxPicture) throw new Error(t('reason.bad_picture'))
    const { texts } = await core.core().call(MethodQRRead, { image: await base64Of(picture) })
    const link = presetLinkIn(texts)
    if (!link) throw new Error(t('settings.presets.noCode'))
    return link
  }

  async function add(find: () => Promise<string>, fromFile = false): Promise<Preset | null> {
    if (busy.value) return null
    busy.value = true
    error.value = ''
    try {
      const link = await find()
      const preview = await core.core().call(MethodPresetsRead, { link })
      if (preview.same) {
        taken.value = { ...preview, link }
        return null
      }
      return await core.core().call(MethodPresetsImport, { link })
    } catch (failed) {
      error.value = fromFile && reasonOf(failed) === ReasonInvalidLink ? t('settings.presets.notPresetFile') : failure(failed)
      return null
    } finally {
      busy.value = false
    }
  }

  const fromText = (text: string) => add(() => Promise.resolve(text.trim()))

  function fromFile(file: File): Promise<Preset | null> {
    if (isPresetFile(file)) {
      return add(async () => {
        if (file.size > maxPresetFile) throw new Error(t('settings.presets.notPresetFile'))
        return file.text()
      }, true)
    }
    if (pictureTypes.includes(file.type)) return add(() => linkOnPicture(file))
    error.value = t('settings.presets.wrongFile')
    return Promise.resolve(null)
  }

  return { busy, error, taken, fromText, fromFile }
}

/** Keeps a file dropped outside every drop zone from opening in the window in place of Poros. */
export function refuseStrayDrops(): void {
  window.addEventListener('dragover', (event) => {
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'none'
  })
  window.addEventListener('drop', (event) => event.preventDefault())
}
