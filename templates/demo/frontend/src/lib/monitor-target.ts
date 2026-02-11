export type MonitorTargetFields = {
  target?: string
  target_url?: string
  target_host?: string
  target_port?: number
  target_record_type?: string
  target_keyword?: string
  target_expected?: string
  target_container?: string
  target_docker_host?: string
  target_push_token?: string
}

export function normalizeTargetFields(
  monitorType: string,
  target: string,
): MonitorTargetFields {
  const type = (monitorType || '').trim().toLowerCase()
  const value = (target || '').trim()
  const out: MonitorTargetFields = {}
  if (!value) return out

  // Compatibility fallback for legacy rows that still only have canonical target.
  if (type === 'http' || type === 'websocket' || type === 'http_keyword' || type === 'http_json_query') {
    let urlPart = value
    if (type === 'http_keyword') {
      const parts = value.split('|')
      if (parts.length >= 2) {
        urlPart = parts[0].trim()
        out.target_keyword = parts.slice(1).join('|').trim()
      }
    } else if (type === 'http_json_query') {
      const parts = value.split('|')
      if (parts.length >= 3) {
        urlPart = parts[0].trim()
        out.target_keyword = parts[1].trim()
        out.target_expected = parts.slice(2).join('|').trim()
      }
    }
    try {
      const u = new URL(urlPart)
      if (!u.hostname) return out
      out.target_url = urlPart
      return out
    } catch {
      return out
    }
  }

  if (type === 'tcp' || type === 'steam' || type === 'tls') {
    const idx = value.lastIndexOf(':')
    if (idx > 0 && idx < value.length-1) {
      out.target_host = value.slice(0, idx).trim()
      const port = Number(value.slice(idx+1).trim())
      if (Number.isFinite(port) && port > 0) out.target_port = port
    } else {
      out.target_host = value
    }
    if (type === 'tls' && !out.target_port) out.target_port = 443
    return out
  }

  if (type === 'ping') {
    out.target_host = value
    return out
  }

  if (type === 'dns') {
    const parts = value.split('|')
    out.target_host = (parts[0] || '').trim()
    out.target_record_type = ((parts[1] || 'A').trim() || 'A').toUpperCase()
    return out
  }

  if (type === 'docker') {
    const parts = value.split('|')
    out.target_container = (parts[0] || '').trim()
    out.target_docker_host = (parts[1] || '').trim()
    return out
  }

  if (type === 'push') {
    out.target_push_token = value
    return out
  }

  return out
}

export function canonicalTargetFromFields(
  monitorType: string,
  fields: MonitorTargetFields,
): string {
  const type = (monitorType || '').trim().toLowerCase()
  const targetURL = (fields.target_url || '').trim()
  const host = (fields.target_host || '').trim()
  const port = Number(fields.target_port || 0)

  if (type === 'http' || type === 'websocket') {
    return targetURL
  }

  if (type === 'http_keyword') {
    const keyword = (fields.target_keyword || '').trim()
    return targetURL && keyword ? `${targetURL}|${keyword}` : ''
  }

  if (type === 'http_json_query') {
    const expr = (fields.target_keyword || '').trim()
    const expected = (fields.target_expected || '').trim()
    return targetURL && expr && expected ? `${targetURL}|${expr}|${expected}` : ''
  }

  if (type === 'tcp' || type === 'steam') {
    return host && port > 0 ? `${host}:${port}` : ''
  }

  if (type === 'tls') {
    if (!host) return ''
    return `${host}:${port > 0 ? port : 443}`
  }

  if (type === 'ping') return host

  if (type === 'dns') {
    if (!host) return ''
    return `${host}|${((fields.target_record_type || 'A').trim() || 'A').toUpperCase()}`
  }

  if (type === 'docker') {
    const container = (fields.target_container || '').trim()
    const dockerHost = (fields.target_docker_host || '').trim()
    if (!container) return ''
    return dockerHost ? `${container}|${dockerHost}` : container
  }

  if (type === 'push') {
    return (fields.target_push_token || '').trim()
  }

  return ''
}

export function displayTargetFromFields(
  monitorType: string,
  fields: MonitorTargetFields,
): string {
  const type = (monitorType || '').trim().toLowerCase()
  const targetURL = (fields.target_url || '').trim()
  const host = (fields.target_host || '').trim()
  const port = Number(fields.target_port || 0)
  const recordType = ((fields.target_record_type || 'A').trim() || 'A').toUpperCase()
  const keyword = (fields.target_keyword || '').trim()
  const expected = (fields.target_expected || '').trim()
  const container = (fields.target_container || '').trim()
  const dockerHost = (fields.target_docker_host || '').trim()
  const pushToken = (fields.target_push_token || '').trim()
  const canonical = (fields.target || '').trim()

  if (type === 'http' || type === 'websocket') return targetURL || canonical
  if (type === 'http_keyword') return targetURL || canonical
  if (type === 'http_json_query') return targetURL || canonical
  if (type === 'tcp' || type === 'steam') return host && port > 0 ? `${host}:${port}` : canonical
  if (type === 'tls') return host ? `${host}:${port > 0 ? port : 443}` : canonical
  if (type === 'ping') return host || canonical
  if (type === 'dns') return host ? `${host} (${recordType})` : canonical
  if (type === 'docker') {
    if (!container) return canonical
    return dockerHost ? `${container} @ ${dockerHost}` : container
  }
  if (type === 'push') return pushToken || canonical
  return canonical
}
