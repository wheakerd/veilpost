import { reactive } from 'vue'

// In-memory only; counts actual requests without introducing artificial delays.
export const authActivity = reactive({ pending: 0 })
