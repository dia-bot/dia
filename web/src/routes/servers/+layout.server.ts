import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/public';
import type { LayoutServerLoad } from './$types';

const API = env.PUBLIC_API_URL ?? 'http://localhost:8080';

export const load: LayoutServerLoad = ({ locals }) => {
	if (!locals.user) {
		throw redirect(303, `${API}/auth/login`);
	}
	return { user: locals.user };
};
