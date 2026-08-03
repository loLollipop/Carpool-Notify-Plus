import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { invalidateAppData } from "./queries"

interface AppMutationOptions<TData, TVariables> {
  /** Show a success toast using the server message (default true). */
  successToast?: boolean
  /** Client-side toast text; overrides the server message when set. */
  successMessage?: string
  onSuccess?: (data: TData, variables: TVariables) => void
  onError?: (error: Error, variables: TVariables) => void
}

/**
 * Wraps a mutation endpoint: invalidates all data queries on success,
 * toasts the server-provided message, and toasts errors.
 */
export function useAppMutation<TVariables = void, TData = { message?: string }>(
  mutationFn: (variables: TVariables) => Promise<TData>,
  options: AppMutationOptions<TData, TVariables> = {},
) {
  const queryClient = useQueryClient()

  return useMutation<TData, Error, TVariables>({
    mutationFn,
    onSuccess: (data, variables) => {
      void invalidateAppData(queryClient)
      if (options.successToast !== false) {
        const serverMessage = (data as { message?: string } | null | undefined)?.message
        const message = options.successMessage ?? serverMessage
        if (message) toast.success(message)
      }
      options.onSuccess?.(data, variables)
    },
    onError: (error, variables) => {
      toast.error(error.message)
      options.onError?.(error, variables)
    },
  })
}
