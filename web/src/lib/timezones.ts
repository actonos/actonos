/**
 * Comprehensive IANA Timezone Helper for ActonOS
 * Supports all 400+ standard global timezones grouped by geographic region,
 * formatted with live UTC offsets.
 */

export interface TimezoneOption {
  value: string;
  label: string;
  region: string;
  offset: string;
}

export interface TimezoneGroup {
  region: string;
  zones: TimezoneOption[];
}

/**
 * Detects the user's current local timezone from the browser environment.
 */
export function detectUserTimezone(): string {
  try {
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    if (tz && typeof tz === 'string') {
      return tz;
    }
  } catch {
    // Fallback
  }
  return 'UTC';
}

/**
 * Returns the complete list of all global IANA timezones, grouped by region.
 */
export function getGroupedTimezones(): TimezoneGroup[] {
  let allTimezones: string[] = [];

  try {
    if (typeof Intl.supportedValuesOf === 'function') {
      allTimezones = Intl.supportedValuesOf('timeZone');
    }
  } catch {
    // Fall through to fallback list
  }

  if (!allTimezones || allTimezones.length === 0) {
    allTimezones = [
      'UTC',
      'Africa/Cairo', 'Africa/Johannesburg', 'Africa/Lagos', 'Africa/Nairobi',
      'America/Argentina/Buenos_Aires', 'America/Bogota', 'America/Chicago',
      'America/Denver', 'America/Los_Angeles', 'America/Mexico_City',
      'America/New_York', 'America/Phoenix', 'America/Santiago',
      'America/Sao_Paulo', 'America/Toronto', 'America/Vancouver',
      'Asia/Bangkok', 'Asia/Dubai', 'Asia/Hong_Kong', 'Asia/Ho_Chi_Minh',
      'Asia/Jakarta', 'Asia/Jerusalem', 'Asia/Kolkata', 'Asia/Manila',
      'Asia/Riyadh', 'Asia/Seoul', 'Asia/Shanghai', 'Asia/Singapore',
      'Asia/Taipei', 'Asia/Tokyo',
      'Atlantic/Reykjavik',
      'Australia/Adelaide', 'Australia/Brisbane', 'Australia/Melbourne',
      'Australia/Perth', 'Australia/Sydney',
      'Europe/Amsterdam', 'Europe/Berlin', 'Europe/Brussels', 'Europe/Dublin',
      'Europe/Helsinki', 'Europe/Istanbul', 'Europe/Lisbon', 'Europe/London',
      'Europe/Madrid', 'Europe/Moscow', 'Europe/Paris', 'Europe/Rome',
      'Europe/Stockholm', 'Europe/Vienna', 'Europe/Warsaw', 'Europe/Zurich',
      'Pacific/Auckland', 'Pacific/Fiji', 'Pacific/Honolulu', 'Pacific/Guam',
    ];
  }

  // Ensure UTC is included at the top
  if (!allTimezones.includes('UTC')) {
    allTimezones.unshift('UTC');
  }

  const now = new Date();
  const groupsMap = new Map<string, TimezoneOption[]>();

  for (const tz of allTimezones) {
    const parts = tz.split('/');
    const region = parts.length > 1 ? parts[0] : 'Global / Universal';
    const city = parts.length > 1 ? parts.slice(1).join(' - ').replace(/_/g, ' ') : tz;

    let offsetStr = '';
    try {
      const formatter = new Intl.DateTimeFormat('en-US', {
        timeZone: tz,
        timeZoneName: 'shortOffset',
      });
      const formattedParts = formatter.formatToParts(now);
      const offsetPart = formattedParts.find((p) => p.type === 'timeZoneName');
      offsetStr = offsetPart ? offsetPart.value : '';
    } catch {
      offsetStr = '';
    }

    const label = offsetStr ? `${city} (${offsetStr})` : city;

    if (!groupsMap.has(region)) {
      groupsMap.set(region, []);
    }
    groupsMap.get(region)!.push({
      value: tz,
      label,
      region,
      offset: offsetStr,
    });
  }

  const groups: TimezoneGroup[] = [];
  
  // Place Global / UTC / Asia / Europe / America / Australia / Africa / Pacific in logical order
  const priorityRegions = ['Global / Universal', 'Asia', 'Europe', 'America', 'Australia', 'Pacific', 'Africa', 'Atlantic'];
  
  for (const pr of priorityRegions) {
    if (groupsMap.has(pr)) {
      groups.push({
        region: pr,
        zones: groupsMap.get(pr)!.sort((a, b) => a.label.localeCompare(b.label)),
      });
      groupsMap.delete(pr);
    }
  }

  // Append any remaining regions alphabetically
  for (const [region, zones] of Array.from(groupsMap.entries()).sort((a, b) => a[0].localeCompare(b[0]))) {
    groups.push({
      region,
      zones: zones.sort((a, b) => a.label.localeCompare(b.label)),
    });
  }

  return groups;
}
