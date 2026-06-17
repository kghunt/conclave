// 12 color schemes: [background, text]
const SCHEMES: [string, string][] = [
	['#e8541e', '#ffffff'], // blurple
	['#38b2c8', '#ffffff'], // sky
	['#57F287', '#111113'], // green
	['#FEE75C', '#111113'], // yellow
	['#EB459E', '#ffffff'], // fuchsia
	['#e04545', '#ffffff'], // red
	['#F4900C', '#ffffff'], // orange
	['#1ABC9C', '#ffffff'], // teal
	['#9B59B6', '#ffffff'], // purple
	['#3498DB', '#ffffff'], // blue
	['#E67E22', '#ffffff'], // dark orange
	['#E91E8C', '#ffffff'], // pink
];

// Stable hash so the same user always gets the same color
function hash(str: string): number {
	let h = 0;
	for (let i = 0; i < str.length; i++) {
		h = (Math.imul(31, h) + str.charCodeAt(i)) | 0;
	}
	return Math.abs(h);
}

export function defaultAvatarUrl(userId: string, displayName: string): string {
	const [bg, fg] = SCHEMES[hash(userId) % SCHEMES.length];
	const letter = (displayName?.[0] ?? '?').toUpperCase();
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40">
  <circle cx="20" cy="20" r="20" fill="${bg}"/>
  <text x="20" y="27" text-anchor="middle" font-family="system-ui,sans-serif"
    font-size="18" font-weight="700" fill="${fg}">${letter}</text>
</svg>`;
	return `data:image/svg+xml;base64,${btoa(svg)}`;
}
