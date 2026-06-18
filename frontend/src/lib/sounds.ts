// Notification sounds via Web Audio API. Each function is self-contained —
// creates a short-lived AudioContext per call so there's no shared state.

function ctx(): AudioContext {
	return new AudioContext();
}

function envelope(gain: GainNode, ac: AudioContext, peakAt: number, peak: number, decay: number) {
	gain.gain.setValueAtTime(0, ac.currentTime);
	gain.gain.linearRampToValueAtTime(peak, ac.currentTime + peakAt);
	gain.gain.exponentialRampToValueAtTime(0.0001, ac.currentTime + peakAt + decay);
}

// Soft single-tone pop — plays when a message arrives in an inactive channel
export function playMessageSound() {
	try {
		const ac = ctx();
		const osc = ac.createOscillator();
		const gain = ac.createGain();
		osc.connect(gain);
		gain.connect(ac.destination);
		osc.type = 'sine';
		osc.frequency.setValueAtTime(700, ac.currentTime);
		osc.frequency.exponentialRampToValueAtTime(500, ac.currentTime + 0.12);
		envelope(gain, ac, 0.005, 0.12, 0.18);
		osc.start(ac.currentTime);
		osc.stop(ac.currentTime + 0.25);
		osc.onended = () => ac.close();
	} catch {}
}

// Ascending two-tone chime — plays on @mention (more noticeable than message)
export function playMentionSound() {
	try {
		const ac = ctx();
		[
			{ freq: 880, delay: 0 },
			{ freq: 1175, delay: 0.13 }
		].forEach(({ freq, delay }) => {
			const osc = ac.createOscillator();
			const gain = ac.createGain();
			osc.connect(gain);
			gain.connect(ac.destination);
			osc.type = 'sine';
			osc.frequency.setValueAtTime(freq, ac.currentTime + delay);
			gain.gain.setValueAtTime(0, ac.currentTime + delay);
			gain.gain.linearRampToValueAtTime(0.22, ac.currentTime + delay + 0.01);
			gain.gain.exponentialRampToValueAtTime(0.0001, ac.currentTime + delay + 0.22);
			osc.start(ac.currentTime + delay);
			osc.stop(ac.currentTime + delay + 0.25);
			osc.onended = () => { try { ac.close(); } catch {} };
		});
	} catch {}
}

// Double-pulse — plays when a DM arrives in an inactive conversation
export function playDMSound() {
	try {
		const ac = ctx();
		[0, 0.14].forEach((delay) => {
			const osc = ac.createOscillator();
			const gain = ac.createGain();
			osc.connect(gain);
			gain.connect(ac.destination);
			osc.type = 'sine';
			osc.frequency.setValueAtTime(660, ac.currentTime + delay);
			gain.gain.setValueAtTime(0, ac.currentTime + delay);
			gain.gain.linearRampToValueAtTime(0.18, ac.currentTime + delay + 0.008);
			gain.gain.exponentialRampToValueAtTime(0.0001, ac.currentTime + delay + 0.15);
			osc.start(ac.currentTime + delay);
			osc.stop(ac.currentTime + delay + 0.18);
			osc.onended = () => { try { ac.close(); } catch {} };
		});
	} catch {}
}
