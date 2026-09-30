/**
 * Derives the issue identifier prefix from a project name.
 *
 * Same rule as the backend:
 * - Split the name on non-alphanumeric characters.
 * - 2+ words → initials of up to 4 words, uppercased ("Frontend App" → "FA").
 * - 1 word → first 4 alphanumeric characters, uppercased ("Backend" → "BACK").
 * - Empty result → "PRJ".
 */
export function projectPrefix(name: string | null | undefined): string {
	const words = (name ?? '').split(/[^\p{L}\p{N}]+/u).filter(Boolean);
	if (words.length === 0) return 'PRJ';
	if (words.length === 1) {
		return words[0].slice(0, 4).toUpperCase() || 'PRJ';
	}
	return (
		words
			.slice(0, 4)
			.map((word) => word[0])
			.join('')
			.toUpperCase() || 'PRJ'
	);
}
