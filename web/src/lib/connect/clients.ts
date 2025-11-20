import { createClient } from '@connectrpc/connect'
import { AccountService } from '@/proto/panel/account_pb.ts'
import { RunService } from '@/proto/panel/run_pb.ts'
import { TemplateService, KvService } from '@/proto/panel/template_pb.ts'
import { appTransport } from './transport'

export const accountClient = createClient(AccountService, appTransport)
export const runClient = createClient(RunService, appTransport)
export const templateClient = createClient(TemplateService, appTransport)
export const templateKvClient = createClient(KvService, appTransport)
