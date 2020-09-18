import { COMMA, ENTER, SPACE } from '@angular/cdk/keycodes';
import { Location } from '@angular/common';
import { Component, OnDestroy, OnInit } from '@angular/core';
import { AbstractControl, FormBuilder, FormControl, FormGroup, ValidatorFn, Validators } from '@angular/forms';
import { MatButtonToggleChange } from '@angular/material/button-toggle';
import { MatChipInputEvent } from '@angular/material/chips';
import { MatDialog } from '@angular/material/dialog';
import { ActivatedRoute, Params } from '@angular/router';
import { TranslateService } from '@ngx-translate/core';
import { Subscription } from 'rxjs';
import {
    Application,
    AppState,
    OIDCApplicationType,
    OIDCAuthMethodType,
    OIDCConfig,
    OIDCGrantType,
    OIDCResponseType,
    ZitadelDocs,
} from 'src/app/proto/generated/management_pb';
import { ManagementService } from 'src/app/services/mgmt.service';
import { ToastService } from 'src/app/services/toast.service';

import { AppSecretDialogComponent } from '../app-secret-dialog/app-secret-dialog.component';

enum RedirectType {
    REDIRECT = 'redirect',
    POSTREDIRECT = 'postredirect',
}

@Component({
    selector: 'app-app-detail',
    templateUrl: './app-detail.component.html',
    styleUrls: ['./app-detail.component.scss'],
})
export class AppDetailComponent implements OnInit, OnDestroy {
    public errorMessage: string = '';
    public removable: boolean = true;
    public addOnBlur: boolean = true;
    public readonly separatorKeysCodes: number[] = [ENTER, COMMA, SPACE];

    private subscription?: Subscription;
    public projectId: string = '';
    public app!: Application.AsObject;
    public oidcResponseTypes: OIDCResponseType[] = [
        OIDCResponseType.OIDCRESPONSETYPE_CODE,
        OIDCResponseType.OIDCRESPONSETYPE_ID_TOKEN,
        OIDCResponseType.OIDCRESPONSETYPE_ID_TOKEN_TOKEN,
    ];
    public oidcGrantTypes: OIDCGrantType[] = [
        OIDCGrantType.OIDCGRANTTYPE_AUTHORIZATION_CODE,
        OIDCGrantType.OIDCGRANTTYPE_IMPLICIT,
        OIDCGrantType.OIDCGRANTTYPE_REFRESH_TOKEN,
    ];
    public oidcAppTypes: OIDCApplicationType[] = [
        OIDCApplicationType.OIDCAPPLICATIONTYPE_WEB,
        OIDCApplicationType.OIDCAPPLICATIONTYPE_USER_AGENT,
        OIDCApplicationType.OIDCAPPLICATIONTYPE_NATIVE,
    ];

    public oidcAuthMethodType: OIDCAuthMethodType[] = [
        OIDCAuthMethodType.OIDCAUTHMETHODTYPE_BASIC,
        OIDCAuthMethodType.OIDCAUTHMETHODTYPE_POST,
        OIDCAuthMethodType.OIDCAUTHMETHODTYPE_NONE,
    ];

    public AppState: any = AppState;
    public appNameForm!: FormGroup;
    public appForm!: FormGroup;
    public redirectUrisList: string[] = [];
    public postLogoutRedirectUrisList: string[] = [];

    public RedirectType: any = RedirectType;

    public isZitadel: boolean = false;
    public docs!: ZitadelDocs.AsObject;

    public OIDCApplicationType: any = OIDCApplicationType;
    public OIDCAuthMethodType: any = OIDCAuthMethodType;

    public redirectControl: FormControl = new FormControl('');
    public postRedirectControl: FormControl = new FormControl('');


    constructor(
        public translate: TranslateService,
        private route: ActivatedRoute,
        private toast: ToastService,
        private fb: FormBuilder,
        private _location: Location,
        private dialog: MatDialog,
        private mgmtService: ManagementService,
    ) {
        this.appNameForm = this.fb.group({
            state: ['', []],
            name: ['', [Validators.required]],
        });
        this.appForm = this.fb.group({
            devMode: [false, []],
            clientId: [{ value: '', disabled: true }],
            responseTypesList: [],
            grantTypesList: [],
            applicationType: [],
            authMethodType: [],
        });
    }

    public ngOnInit(): void {
        this.subscription = this.route.params.subscribe(params => this.getData(params));
    }

    public ngOnDestroy(): void {
        this.subscription?.unsubscribe();
    }

    private async getData({ projectid, id }: Params): Promise<void> {
        this.projectId = projectid;
        this.mgmtService.GetIam().then(iam => {
            this.isZitadel = iam.toObject().iamProjectId === this.projectId;
        });

        this.mgmtService.GetApplicationById(projectid, id).then(app => {
            this.app = app.toObject();
            this.appNameForm.patchValue(this.app);

            if (this.app.state !== AppState.APPSTATE_ACTIVE) {
                this.appNameForm.controls['name'].disable();
                this.appForm.disable();
            } else {
                this.appNameForm.controls['name'].enable();
                this.appForm.enable();
            }
            if (this.app.oidcConfig?.redirectUrisList) {
                this.redirectUrisList = this.app.oidcConfig.redirectUrisList;

                this.redirectControl = new FormControl('');
            }
            if (this.app.oidcConfig?.postLogoutRedirectUrisList) {
                this.postLogoutRedirectUrisList = this.app.oidcConfig.postLogoutRedirectUrisList;
                this.postRedirectControl = new FormControl('');
            }
            if (this.app.oidcConfig) {
                this.appForm.patchValue(this.app.oidcConfig);
            }
        }).catch(error => {
            console.error(error);
            this.toast.showError(error);
            this.errorMessage = error.message;
        });

        this.docs = (await this.mgmtService.GetZitadelDocs()).toObject();
    }

    public changeState(event: MatButtonToggleChange): void {
        if (event.value === AppState.APPSTATE_ACTIVE) {
            this.mgmtService.ReactivateApplication(this.projectId, this.app.id).then(() => {
                this.toast.showInfo('APP.TOAST.REACTIVATED', true);
            }).catch((error: any) => {
                this.toast.showError(error);
            });
        } else if (event.value === AppState.APPSTATE_INACTIVE) {
            this.mgmtService.DeactivateApplication(this.projectId, this.app.id).then(() => {
                this.toast.showInfo('APP.TOAST.REACTIVATED', true);
            }).catch((error: any) => {
                this.toast.showError(error);
            });
        }

        if (event.value !== AppState.APPSTATE_ACTIVE) {
            this.appNameForm.controls['name'].disable();
            this.appForm.disable();
        } else {
            this.appNameForm.controls['name'].enable();
            this.appForm.enable();
            this.clientId?.disable();
        }
    }

    public add(event: MatChipInputEvent, target: RedirectType): void {
        if (target === RedirectType.POSTREDIRECT && this.postRedirectControl.valid) {
            const input = event.input;
            if (event.value !== '' && event.value !== ' ' && event.value !== '/') {
                this.postLogoutRedirectUrisList.push(event.value);
            }
            if (input) {
                input.value = '';
            }
        } else if (target === RedirectType.REDIRECT && this.redirectControl.valid) {
            const input = event.input;
            if (event.value !== '' && event.value !== ' ' && event.value !== '/') {
                this.redirectUrisList.push(event.value);
            }
            if (input) {
                input.value = '';
            }
        }
    }

    public remove(redirect: any, target: RedirectType): void {
        if (target === RedirectType.POSTREDIRECT) {
            const index = this.postLogoutRedirectUrisList.indexOf(redirect);

            if (index >= 0) {
                this.postLogoutRedirectUrisList.splice(index, 1);
            }
        } else if (target === RedirectType.REDIRECT) {
            const index = this.redirectUrisList.indexOf(redirect);

            if (index >= 0) {
                this.redirectUrisList.splice(index, 1);
            }
        }
    }

    public saveOIDCApp(): void {
        if (this.appNameForm.valid) {
            this.app.name = this.name?.value;
        }

        if (this.appForm.valid) {
            if (this.app.oidcConfig) {
                this.app.oidcConfig.responseTypesList = this.responseTypesList?.value;
                this.app.oidcConfig.grantTypesList = this.grantTypesList?.value;
                this.app.oidcConfig.applicationType = this.applicationType?.value;
                this.app.oidcConfig.authMethodType = this.authMethodType?.value;
                this.app.oidcConfig.redirectUrisList = this.redirectUrisList;
                this.app.oidcConfig.postLogoutRedirectUrisList = this.postLogoutRedirectUrisList;
                this.app.oidcConfig.devMode = this.devMode?.value;

                this.mgmtService
                    .UpdateOIDCAppConfig(this.projectId, this.app.id, this.app.oidcConfig)
                    .then(() => {
                        this.toast.showInfo('APP.TOAST.OIDCUPDATED', true);
                    })
                    .catch(error => {
                        this.toast.showError(error);
                    });
            }
        }
    }

    public regenerateOIDCClientSecret(): void {
        this.mgmtService.RegenerateOIDCClientSecret(this.app.id, this.projectId).then((data: OIDCConfig) => {
            this.toast.showInfo('APP.TOAST.OIDCCLIENTSECRETREGENERATED', true);
            this.dialog.open(AppSecretDialogComponent, {
                data: {
                    clientId: data.toObject().clientId,
                    clientSecret: data.toObject().clientSecret,
                },
                width: '400px',
            });

        }).catch(error => {
            this.toast.showError(error);
        });
    }

    public navigateBack(): void {
        this._location.back();
    }

    public get name(): AbstractControl | null {
        return this.appNameForm.get('name');
    }

    public get clientId(): AbstractControl | null {
        return this.appForm.get('clientId');
    }

    public get responseTypesList(): AbstractControl | null {
        return this.appForm.get('responseTypesList');
    }

    public get grantTypesList(): AbstractControl | null {
        return this.appForm.get('grantTypesList');
    }

    public get applicationType(): AbstractControl | null {
        return this.appForm.get('applicationType');
    }

    public get authMethodType(): AbstractControl | null {
        return this.appForm.get('authMethodType');
    }

    public get devMode(): AbstractControl | null {
        return this.appForm.get('devMode');
    }
}
