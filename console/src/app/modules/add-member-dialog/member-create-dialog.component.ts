import { Component, Inject } from '@angular/core';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { ProjectGrantView, ProjectRole, ProjectView, UserView } from 'src/app/proto/generated/management_pb';
import { AdminService } from 'src/app/services/admin.service';
import { ManagementService } from 'src/app/services/mgmt.service';
import { ToastService } from 'src/app/services/toast.service';

import { ProjectAutocompleteType } from '../search-project-autocomplete/search-project-autocomplete.component';

export enum CreationType {
    PROJECT_OWNED = 0,
    PROJECT_GRANTED = 1,
    ORG = 2,
    IAM = 3,
}
@Component({
    selector: 'app-member-create-dialog',
    templateUrl: './member-create-dialog.component.html',
    styleUrls: ['./member-create-dialog.component.scss'],
})
export class MemberCreateDialogComponent {
    private projectId: string = '';
    private grantId: string = '';
    public preselectedUsers: Array<UserView.AsObject> = [];


    public creationType!: CreationType;
    public creationTypes: CreationType[] = [
        CreationType.IAM,
        CreationType.ORG,
        CreationType.PROJECT_OWNED,
        CreationType.PROJECT_GRANTED,
    ];
    public users: Array<UserView.AsObject> = [];
    public roles: Array<ProjectRole.AsObject> | string[] = [];
    public CreationType: any = CreationType;
    public ProjectAutocompleteType: any = ProjectAutocompleteType;
    public memberRoleOptions: string[] = [];

    public showCreationTypeSelector: boolean = false;
    constructor(
        private mgmtService: ManagementService,
        private adminService: AdminService,
        public dialogRef: MatDialogRef<MemberCreateDialogComponent>,
        @Inject(MAT_DIALOG_DATA) public data: any,
        private toastService: ToastService,
    ) {
        if (data?.projectId) {
            this.projectId = data.projectId;
        }
        if (data?.user) {
            this.preselectedUsers = [data.user];
            this.users = [data.user];
        }

        if (data?.creationType !== undefined) {
            this.creationType = data.creationType;
            this.loadRoles();
        } else {
            this.showCreationTypeSelector = true;
        }
    }

    public loadRoles(): void {
        switch (this.creationType) {
            case CreationType.PROJECT_GRANTED:
                this.mgmtService.GetProjectGrantMemberRoles().then(resp => {
                    this.memberRoleOptions = resp.toObject().rolesList;
                }).catch(error => {
                    this.toastService.showError(error);
                });
                break;
            case CreationType.PROJECT_OWNED:
                this.mgmtService.GetProjectMemberRoles().then(resp => {
                    this.memberRoleOptions = resp.toObject().rolesList;
                }).catch(error => {
                    this.toastService.showError(error);
                });
                break;
            case CreationType.IAM:
                this.adminService.GetIamMemberRoles().then(resp => {
                    this.memberRoleOptions = resp.toObject().rolesList;
                }).catch(error => {
                    this.toastService.showError(error);
                });
                break;
        }
    }

    public selectProject(project: ProjectView.AsObject | ProjectGrantView.AsObject | any): void {
        this.projectId = project.projectId;
        if (project.id) {
            this.grantId = project.id;
        }
    }

    public closeDialog(): void {
        this.dialogRef.close(false);
    }

    public closeDialogWithSuccess(): void {
        this.dialogRef.close({
            users: this.users,
            roles: this.roles,
            creationType: this.creationType,
            projectId: this.projectId,
            grantId: this.grantId,
        });
    }

    public setOrgMemberRoles(roles: string[]): void {
        this.roles = roles;
    }
}
