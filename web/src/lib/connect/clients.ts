import { createClient } from '@connectrpc/connect'
import { AccountService } from '@/proto/panel/account_pb.ts'
import { RunService } from '@/proto/panel/run_pb.ts'
import { AutomateService, ResourcesService } from '@/proto/panel/automate_pb.ts'
import { appTransport } from './transport'

export const accountClient = createClient(AccountService, appTransport)
export const runClient = createClient(RunService, appTransport)
export const automateClient = createClient(AutomateService, appTransport)
export const resourcesClient = createClient(ResourcesService, appTransport)
