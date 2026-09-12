import { z } from "zod";
export declare const DEFAULT_BASE_URL = "https://exchange.lamdis.ai";
export interface LamdisOptions {
    /** Another exchange, e.g. a private deployment. Defaults to LAMDIS_BASE_URL or exchange.lamdis.ai. */
    baseUrl?: string;
    /** A fetch to use instead of the global one. */
    fetch?: typeof fetch;
}
/** The shape every tool returns: the exchange's JSON, or an error with the HTTP status. */
export type LamdisResult = Record<string, unknown> & {
    error?: string;
    http_status?: number;
};
export interface QuoteInput {
    predicate: string;
    lat: number;
    lon: number;
    kind?: "observe" | "do";
    skills?: string[];
    sandbox?: boolean;
}
/** POST /v1/quote: can anybody take this, would it be refused, what has it cost. */
export declare function quote(input: QuoteInput, opts?: LamdisOptions): Promise<LamdisResult>;
export interface PostTaskInput {
    predicate: string;
    lat: number;
    lon: number;
    fee_minor: number;
    kind?: "observe" | "do";
    radius_m?: number;
    where?: string;
    area?: string;
    instructions?: string;
    deliverable?: string;
    skills?: string[];
    attempt_minor?: number;
    /** Default true. */
    sandbox?: boolean;
}
/** POST /v1/tasks with no credential: sandbox, or a real job that comes back with pay_at. */
export declare function postTask(input: PostTaskInput, opts?: LamdisOptions): Promise<LamdisResult>;
/** GET /v1/jobs/{job} with the lbt_ token that came back when it was posted. */
export declare function jobStatus(job: string, token: string, opts?: LamdisOptions): Promise<LamdisResult>;
/** GET /v1/jobs/{job}/receipt: the signed receipt once the job has settled. */
export declare function jobReceipt(job: string, token: string, opts?: LamdisOptions): Promise<LamdisResult>;
export declare const checkFeasibleSchema: z.ZodObject<{
    predicate: z.ZodString;
    lat: z.ZodNumber;
    lon: z.ZodNumber;
    kind: z.ZodDefault<z.ZodEnum<{
        observe: "observe";
        do: "do";
    }>>;
    skills: z.ZodOptional<z.ZodArray<z.ZodString>>;
    sandbox: z.ZodDefault<z.ZodBoolean>;
}, z.core.$strip>;
export declare const runJobSchema: z.ZodObject<{
    predicate: z.ZodString;
    lat: z.ZodNumber;
    lon: z.ZodNumber;
    fee_minor: z.ZodNumber;
    kind: z.ZodDefault<z.ZodEnum<{
        observe: "observe";
        do: "do";
    }>>;
    radius_m: z.ZodDefault<z.ZodNumber>;
    where: z.ZodOptional<z.ZodString>;
    instructions: z.ZodOptional<z.ZodString>;
    deliverable: z.ZodOptional<z.ZodString>;
    sandbox: z.ZodDefault<z.ZodBoolean>;
}, z.core.$strip>;
export declare const jobStatusSchema: z.ZodObject<{
    job: z.ZodString;
    token: z.ZodString;
}, z.core.$strip>;
/** Build the three tools against a particular exchange. `lamdisTools` is this with defaults. */
export declare function createLamdisTools(opts?: LamdisOptions): {
    lamdis_check_feasible: ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type?: undefined | "function";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type: "dynamic";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: false;
        supportsDeferredResults?: never;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: true;
        supportsDeferredResults?: boolean;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    });
    lamdis_run_job: ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type?: undefined | "function";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type: "dynamic";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: false;
        supportsDeferredResults?: never;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: true;
        supportsDeferredResults?: boolean;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    });
    lamdis_job_status: ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            job: string;
            token: string;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            job: string;
            token: string;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                job: string;
                token: string;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                job: string;
                token: string;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                job: string;
                token: string;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type?: undefined | "function";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            job: string;
            token: string;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            job: string;
            token: string;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                job: string;
                token: string;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                job: string;
                token: string;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                job: string;
                token: string;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type: "dynamic";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            job: string;
            token: string;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            job: string;
            token: string;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                job: string;
                token: string;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                job: string;
                token: string;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: false;
        supportsDeferredResults?: never;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            job: string;
            token: string;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            job: string;
            token: string;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                job: string;
                token: string;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                job: string;
                token: string;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: true;
        supportsDeferredResults?: boolean;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    });
};
/** The three tools against exchange.lamdis.ai. Spread into `tools:` or pass as-is. */
export declare const lamdisTools: {
    lamdis_check_feasible: ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type?: undefined | "function";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type: "dynamic";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: false;
        supportsDeferredResults?: never;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                kind: "observe" | "do";
                sandbox: boolean;
                skills?: string[] | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: true;
        supportsDeferredResults?: boolean;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    });
    lamdis_run_job: ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type?: undefined | "function";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type: "dynamic";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: false;
        supportsDeferredResults?: never;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                predicate: string;
                lat: number;
                lon: number;
                fee_minor: number;
                kind: "observe" | "do";
                radius_m: number;
                sandbox: boolean;
                where?: string | undefined;
                instructions?: string | undefined;
                deliverable?: string | undefined;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: true;
        supportsDeferredResults?: boolean;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    });
    lamdis_job_status: ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            job: string;
            token: string;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            job: string;
            token: string;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                job: string;
                token: string;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                job: string;
                token: string;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                job: string;
                token: string;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type?: undefined | "function";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            job: string;
            token: string;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            job: string;
            token: string;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                job: string;
                token: string;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                job: string;
                token: string;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        description?: string | ((options: {
            context: NoInfer<import("@ai-sdk/provider-utils").Context>;
            experimental_sandbox?: import("ai").Experimental_SandboxSession;
        }) => string) | undefined;
        strict?: boolean;
        inputExamples?: {
            input: NoInfer<{
                job: string;
                token: string;
            }>;
        }[] | undefined;
        id?: never;
        isProviderExecuted?: never;
        args?: never;
        supportsDeferredResults?: never;
    } & {
        type: "dynamic";
    } & {
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            job: string;
            token: string;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            job: string;
            token: string;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                job: string;
                token: string;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                job: string;
                token: string;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: false;
        supportsDeferredResults?: never;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    }) | ({
        title?: string;
        providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
        metadata?: import("@ai-sdk/provider").JSONObject;
        inputSchema: import("ai").FlexibleSchema<{
            job: string;
            token: string;
        }>;
        contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
        needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
            job: string;
            token: string;
        }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
        onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputDelta?: ((options: {
            inputTextDelta: string;
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        onInputAvailable?: ((options: {
            input: {
                job: string;
                token: string;
            };
        } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
        toModelOutput?: ((options: {
            toolCallId: string;
            input: {
                job: string;
                token: string;
            };
            output: NoInfer<LamdisResult>;
        }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
    } & {
        outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    } & {
        type: "provider";
        id: `${string}.${string}`;
        args: Record<string, unknown>;
        description?: never;
        strict?: never;
        inputExamples?: never;
    } & {
        isProviderExecuted: true;
        supportsDeferredResults?: boolean;
    } & {
        execute: import("ai").ToolExecuteFunction<{
            job: string;
            token: string;
        }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
    });
};
export declare const lamdisCheckFeasible: ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    description?: string | ((options: {
        context: NoInfer<import("@ai-sdk/provider-utils").Context>;
        experimental_sandbox?: import("ai").Experimental_SandboxSession;
    }) => string) | undefined;
    strict?: boolean;
    inputExamples?: {
        input: NoInfer<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
    }[] | undefined;
    id?: never;
    isProviderExecuted?: never;
    args?: never;
    supportsDeferredResults?: never;
} & {
    type?: undefined | "function";
} & {
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
}) | ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    description?: string | ((options: {
        context: NoInfer<import("@ai-sdk/provider-utils").Context>;
        experimental_sandbox?: import("ai").Experimental_SandboxSession;
    }) => string) | undefined;
    strict?: boolean;
    inputExamples?: {
        input: NoInfer<{
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        }>;
    }[] | undefined;
    id?: never;
    isProviderExecuted?: never;
    args?: never;
    supportsDeferredResults?: never;
} & {
    type: "dynamic";
} & {
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
}) | ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    type: "provider";
    id: `${string}.${string}`;
    args: Record<string, unknown>;
    description?: never;
    strict?: never;
    inputExamples?: never;
} & {
    isProviderExecuted: false;
    supportsDeferredResults?: never;
} & {
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
}) | ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            predicate: string;
            lat: number;
            lon: number;
            kind: "observe" | "do";
            sandbox: boolean;
            skills?: string[] | undefined;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    type: "provider";
    id: `${string}.${string}`;
    args: Record<string, unknown>;
    description?: never;
    strict?: never;
    inputExamples?: never;
} & {
    isProviderExecuted: true;
    supportsDeferredResults?: boolean;
} & {
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        kind: "observe" | "do";
        sandbox: boolean;
        skills?: string[] | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
});
export declare const lamdisRunJob: ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    description?: string | ((options: {
        context: NoInfer<import("@ai-sdk/provider-utils").Context>;
        experimental_sandbox?: import("ai").Experimental_SandboxSession;
    }) => string) | undefined;
    strict?: boolean;
    inputExamples?: {
        input: NoInfer<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
    }[] | undefined;
    id?: never;
    isProviderExecuted?: never;
    args?: never;
    supportsDeferredResults?: never;
} & {
    type?: undefined | "function";
} & {
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
}) | ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    description?: string | ((options: {
        context: NoInfer<import("@ai-sdk/provider-utils").Context>;
        experimental_sandbox?: import("ai").Experimental_SandboxSession;
    }) => string) | undefined;
    strict?: boolean;
    inputExamples?: {
        input: NoInfer<{
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        }>;
    }[] | undefined;
    id?: never;
    isProviderExecuted?: never;
    args?: never;
    supportsDeferredResults?: never;
} & {
    type: "dynamic";
} & {
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
}) | ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    type: "provider";
    id: `${string}.${string}`;
    args: Record<string, unknown>;
    description?: never;
    strict?: never;
    inputExamples?: never;
} & {
    isProviderExecuted: false;
    supportsDeferredResults?: never;
} & {
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
}) | ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            predicate: string;
            lat: number;
            lon: number;
            fee_minor: number;
            kind: "observe" | "do";
            radius_m: number;
            sandbox: boolean;
            where?: string | undefined;
            instructions?: string | undefined;
            deliverable?: string | undefined;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    type: "provider";
    id: `${string}.${string}`;
    args: Record<string, unknown>;
    description?: never;
    strict?: never;
    inputExamples?: never;
} & {
    isProviderExecuted: true;
    supportsDeferredResults?: boolean;
} & {
    execute: import("ai").ToolExecuteFunction<{
        predicate: string;
        lat: number;
        lon: number;
        fee_minor: number;
        kind: "observe" | "do";
        radius_m: number;
        sandbox: boolean;
        where?: string | undefined;
        instructions?: string | undefined;
        deliverable?: string | undefined;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
});
export declare const lamdisJobStatus: ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        job: string;
        token: string;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        job: string;
        token: string;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            job: string;
            token: string;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            job: string;
            token: string;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        job: string;
        token: string;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    description?: string | ((options: {
        context: NoInfer<import("@ai-sdk/provider-utils").Context>;
        experimental_sandbox?: import("ai").Experimental_SandboxSession;
    }) => string) | undefined;
    strict?: boolean;
    inputExamples?: {
        input: NoInfer<{
            job: string;
            token: string;
        }>;
    }[] | undefined;
    id?: never;
    isProviderExecuted?: never;
    args?: never;
    supportsDeferredResults?: never;
} & {
    type?: undefined | "function";
} & {
    execute: import("ai").ToolExecuteFunction<{
        job: string;
        token: string;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
}) | ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        job: string;
        token: string;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        job: string;
        token: string;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            job: string;
            token: string;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            job: string;
            token: string;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        job: string;
        token: string;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    description?: string | ((options: {
        context: NoInfer<import("@ai-sdk/provider-utils").Context>;
        experimental_sandbox?: import("ai").Experimental_SandboxSession;
    }) => string) | undefined;
    strict?: boolean;
    inputExamples?: {
        input: NoInfer<{
            job: string;
            token: string;
        }>;
    }[] | undefined;
    id?: never;
    isProviderExecuted?: never;
    args?: never;
    supportsDeferredResults?: never;
} & {
    type: "dynamic";
} & {
    execute: import("ai").ToolExecuteFunction<{
        job: string;
        token: string;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
}) | ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        job: string;
        token: string;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        job: string;
        token: string;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            job: string;
            token: string;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            job: string;
            token: string;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        job: string;
        token: string;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    type: "provider";
    id: `${string}.${string}`;
    args: Record<string, unknown>;
    description?: never;
    strict?: never;
    inputExamples?: never;
} & {
    isProviderExecuted: false;
    supportsDeferredResults?: never;
} & {
    execute: import("ai").ToolExecuteFunction<{
        job: string;
        token: string;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
}) | ({
    title?: string;
    providerOptions?: import("@ai-sdk/provider-utils").ProviderOptions;
    metadata?: import("@ai-sdk/provider").JSONObject;
    inputSchema: import("ai").FlexibleSchema<{
        job: string;
        token: string;
    }>;
    contextSchema?: import("ai").FlexibleSchema<import("@ai-sdk/provider-utils").Context> | undefined;
    needsApproval?: boolean | import("@ai-sdk/provider-utils").ToolNeedsApprovalFunction<{
        job: string;
        token: string;
    }, NoInfer<import("@ai-sdk/provider-utils").Context>> | undefined;
    onInputStart?: ((options: import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputDelta?: ((options: {
        inputTextDelta: string;
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    onInputAvailable?: ((options: {
        input: {
            job: string;
            token: string;
        };
    } & import("ai").ToolExecutionOptions<NoInfer<import("@ai-sdk/provider-utils").Context>>) => void | PromiseLike<void>) | undefined;
    toModelOutput?: ((options: {
        toolCallId: string;
        input: {
            job: string;
            token: string;
        };
        output: NoInfer<LamdisResult>;
    }) => import("@ai-sdk/provider-utils").ToolResultOutput | PromiseLike<import("@ai-sdk/provider-utils").ToolResultOutput>) | undefined;
} & {
    outputSchema?: import("ai").FlexibleSchema<LamdisResult> | undefined;
    execute: import("ai").ToolExecuteFunction<{
        job: string;
        token: string;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
} & {
    type: "provider";
    id: `${string}.${string}`;
    args: Record<string, unknown>;
    description?: never;
    strict?: never;
    inputExamples?: never;
} & {
    isProviderExecuted: true;
    supportsDeferredResults?: boolean;
} & {
    execute: import("ai").ToolExecuteFunction<{
        job: string;
        token: string;
    }, LamdisResult, NoInfer<import("@ai-sdk/provider-utils").Context>>;
});
