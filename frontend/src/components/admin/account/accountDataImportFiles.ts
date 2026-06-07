import JSZip from 'jszip'
import type { AdminDataPayload } from '@/types'

const SUPPORTED_DATA_TYPES = new Set(['sub2api-data', 'sub2api-bundle'])
const SUPPORTED_DATA_VERSION = 1
const ZIP_MIME_TYPES = new Set([
  'application/zip',
  'application/x-zip',
  'application/x-zip-compressed',
  'multipart/x-zip'
])

type ImportDataPayloadCandidate = Partial<AdminDataPayload> & Record<string, unknown>

const isZipFile = (sourceFile: File): boolean => {
  const name = sourceFile.name.toLowerCase()
  const mime = sourceFile.type.toLowerCase()
  return name.endsWith('.zip') || ZIP_MIME_TYPES.has(mime)
}

const readFileAsText = async (sourceFile: File): Promise<string> => {
  if (typeof sourceFile.text === 'function') {
    return sourceFile.text()
  }

  if (typeof sourceFile.arrayBuffer === 'function') {
    const buffer = await sourceFile.arrayBuffer()
    return new TextDecoder().decode(buffer)
  }

  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.readAsText(sourceFile)
  })
}

const readFileAsArrayBuffer = async (sourceFile: File): Promise<ArrayBuffer> => {
  if (typeof sourceFile.arrayBuffer === 'function') {
    return sourceFile.arrayBuffer()
  }

  return await new Promise<ArrayBuffer>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      if (reader.result instanceof ArrayBuffer) {
        resolve(reader.result)
        return
      }
      reject(new Error('Failed to read file as array buffer'))
    }
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.readAsArrayBuffer(sourceFile)
  })
}

function validateDataPayload(
  payload: unknown,
  sourceFileName: string
): asserts payload is ImportDataPayloadCandidate {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
    throw new Error(`${sourceFileName}: invalid data payload`)
  }

  const candidate = payload as ImportDataPayloadCandidate

  const type = typeof candidate.type === 'string' ? candidate.type : ''
  if (type !== '' && !SUPPORTED_DATA_TYPES.has(type)) {
    throw new Error(`${sourceFileName}: unsupported data type: ${type}`)
  }

  if (
    candidate.version !== undefined &&
    candidate.version !== 0 &&
    candidate.version !== SUPPORTED_DATA_VERSION
  ) {
    throw new Error(`${sourceFileName}: unsupported data version: ${String(candidate.version)}`)
  }

  if (!Array.isArray(candidate.proxies)) {
    throw new Error(`${sourceFileName}: proxies is required`)
  }

  if (!Array.isArray(candidate.accounts)) {
    throw new Error(`${sourceFileName}: accounts is required`)
  }
}

const normalizeDataPayload = (payload: ImportDataPayloadCandidate): AdminDataPayload => ({
  type: typeof payload.type === 'string' ? payload.type : undefined,
  version: typeof payload.version === 'number' ? payload.version : undefined,
  exported_at:
    typeof payload.exported_at === 'string' ? payload.exported_at : '',
  proxies: payload.proxies ?? [],
  accounts: payload.accounts ?? []
})

const parseJsonImportPayload = (text: string, sourceFileName: string): AdminDataPayload => {
  const payload = JSON.parse(text)
  validateDataPayload(payload, sourceFileName)
  return normalizeDataPayload(payload)
}

const isZipJsonEntryName = (entryName: string): boolean => {
  const normalized = entryName.replace(/\\/g, '/')
  const fileName = normalized.split('/').pop() || ''
  return (
    normalized.toLowerCase().endsWith('.json') &&
    !normalized.startsWith('__MACOSX/') &&
    !fileName.startsWith('._')
  )
}

const parseZipImportFile = async (sourceFile: File): Promise<AdminDataPayload[]> => {
  const buffer = await readFileAsArrayBuffer(sourceFile)
  const archive = await JSZip.loadAsync(buffer)
  const jsonEntries = Object.values(archive.files).filter((entry) => (
    !entry.dir && isZipJsonEntryName(entry.name)
  ))

  if (jsonEntries.length === 0) {
    throw new Error(`${sourceFile.name}: no JSON files found in zip`)
  }

  const payloads: AdminDataPayload[] = []
  for (const entry of jsonEntries) {
    const text = await entry.async('text')
    payloads.push(parseJsonImportPayload(text, `${sourceFile.name}:${entry.name}`))
  }
  return payloads
}

const parseImportFilePayloads = async (sourceFile: File): Promise<AdminDataPayload[]> => {
  if (isZipFile(sourceFile)) {
    return parseZipImportFile(sourceFile)
  }

  const text = await readFileAsText(sourceFile)
  return [parseJsonImportPayload(text, sourceFile.name)]
}

export const parseImportFiles = async (sourceFiles: File[]): Promise<AdminDataPayload[]> => {
  const payloadGroups = await Promise.all(sourceFiles.map(parseImportFilePayloads))
  return payloadGroups.flat()
}

export const mergeDataPayloads = (payloads: AdminDataPayload[]): AdminDataPayload => ({
  type: 'sub2api-data',
  version: SUPPORTED_DATA_VERSION,
  exported_at: new Date().toISOString(),
  proxies: payloads.flatMap((item) => item.proxies),
  accounts: payloads.flatMap((item) => item.accounts)
})
