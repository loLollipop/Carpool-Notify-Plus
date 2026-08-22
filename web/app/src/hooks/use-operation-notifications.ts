import * as React from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { acknowledgeOperationTasks } from "@/api/endpoints"
import { queryKeys, useOperationsOverview } from "@/api/queries"
import type { OperationTask } from "@/api/types"

export function useOperationNotifications() {
  const overviewQuery = useOperationsOverview()
  const queryClient = useQueryClient()
  const notifications = React.useMemo(
    () => (overviewQuery.data?.notifications ?? []).filter((task) => task.unread),
    [overviewQuery.data?.notifications],
  )
  const mutation = useMutation({
    mutationFn: acknowledgeOperationTasks,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.operationsOverview })
    },
    onError: (error) => toast.error(error.message),
  })

  const acknowledge = React.useCallback(
    (tasks: OperationTask[] | string[]) => {
      const taskIds = tasks.map((task) => (typeof task === "string" ? task : task.id))
      if (taskIds.length > 0) mutation.mutate(taskIds)
    },
    [mutation],
  )

  return { notifications, acknowledge }
}
