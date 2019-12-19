package backup_test

import (
	"github.com/greenplum-db/gp-common-go-libs/operating"
	"github.com/greenplum-db/gp-common-go-libs/structmatcher"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpbackup/backup"
	"github.com/greenplum-db/gpbackup/backup_filepath"
	"github.com/greenplum-db/gpbackup/backup_history"
	"github.com/greenplum-db/gpbackup/testutils"
	"github.com/greenplum-db/gpbackup/utils"
	"github.com/onsi/gomega/gbytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("backup/incremental tests", func() {
	Describe("FilterTablesForIncremental", func() {
		defaultEntry := utils.AOEntry{
			Modcount:         0,
			LastDDLTimestamp: "00000",
		}
		prevTOC := utils.TOC{
			IncrementalMetadata: utils.IncrementalEntries{
				AO: map[string]utils.AOEntry{
					"public.ao_changed_modcount":  defaultEntry,
					"public.ao_changed_timestamp": defaultEntry,
					"public.ao_unchanged":         defaultEntry,
				},
			},
		}

		currTOC := utils.TOC{
			IncrementalMetadata: utils.IncrementalEntries{
				AO: map[string]utils.AOEntry{
					"public.ao_changed_modcount": {
						Modcount:         2,
						LastDDLTimestamp: "00000",
					},
					"public.ao_changed_timestamp": {
						Modcount:         0,
						LastDDLTimestamp: "00001",
					},
					"public.ao_unchanged": defaultEntry,
				},
			},
		}

		tblHeap := backup.Table{Relation: backup.Relation{Schema: "public", Name: "heap"}}
		tblAOChangedModcount := backup.Table{Relation: backup.Relation{Schema: "public", Name: "ao_changed_modcount"}}
		tblAOChangedTS := backup.Table{Relation: backup.Relation{Schema: "public", Name: "ao_changed_timestamp"}}
		tblAOUnchanged := backup.Table{Relation: backup.Relation{Schema: "public", Name: "ao_unchanged"}}
		tables := []backup.Table{
			tblHeap,
			tblAOChangedModcount,
			tblAOChangedTS,
			tblAOUnchanged,
		}

		filteredTables := backup.FilterTablesForIncremental(&prevTOC, &currTOC, tables)

		It("Should include the heap table in the filtered list", func() {
			Expect(filteredTables).To(ContainElement(tblHeap))
		})

		It("Should include the AO table having a modified modcount", func() {
			Expect(filteredTables).To(ContainElement(tblAOChangedModcount))
		})

		It("Should include the AO table having a modified last DDL timestamp", func() {
			Expect(filteredTables).To(ContainElement(tblAOChangedTS))
		})

		It("Should NOT include the unmodified AO table", func() {
			Expect(filteredTables).To(Not(ContainElement(tblAOUnchanged)))
		})
	})

	Describe("GetLatestMatchingBackupConfig", func() {
		history := backup_history.History{BackupConfigs: []backup_history.BackupConfig{
			{DatabaseName: "test2", Timestamp: "timestamp4"},
			{DatabaseName: "test1", Timestamp: "timestamp3"},
			{DatabaseName: "test2", Timestamp: "timestamp2"},
			{DatabaseName: "test1", Timestamp: "timestamp1"},
		}}
		It("Should return the latest backup's timestamp with matching Dbname", func() {
			currentBackupConfig := backup_history.BackupConfig{DatabaseName: "test1"}

			latestBackupHistoryEntry := backup.GetLatestMatchingBackupConfig(&history, &currentBackupConfig)

			structmatcher.ExpectStructsToMatch(history.BackupConfigs[1], latestBackupHistoryEntry)
		})
		It("should return nil with no matching Dbname", func() {
			currentBackupConfig := backup_history.BackupConfig{DatabaseName: "test3"}

			latestBackupHistoryEntry := backup.GetLatestMatchingBackupConfig(&history, &currentBackupConfig)

			Expect(latestBackupHistoryEntry).To(BeNil())
		})
		It("should return nil with an empty history", func() {
			currentBackupConfig := backup_history.BackupConfig{}

			latestBackupHistoryEntry := backup.
				GetLatestMatchingBackupConfig(&backup_history.History{BackupConfigs: []backup_history.BackupConfig{}}, &currentBackupConfig)

			Expect(latestBackupHistoryEntry).To(BeNil())
		})
	})

	Describe("PopulateRestorePlan", func() {
		testCluster := testutils.SetDefaultSegmentConfiguration()
		testFPInfo := backup_filepath.NewFilePathInfo(testCluster, "", "ts0",
			"gpseg")
		backup.SetFPInfo(testFPInfo)

		Context("Full backup", func() {
			restorePlan := make([]backup_history.RestorePlanEntry, 0)
			backupSetTables := []backup.Table{
				{Relation: backup.Relation{Schema: "public", Name: "ao1"}},
				{Relation: backup.Relation{Schema: "public", Name: "heap1"}},
			}
			allTables := backupSetTables

			restorePlan = backup.PopulateRestorePlan(backupSetTables, restorePlan, allTables)

			It("Should populate a restore plan with a single entry", func() {
				Expect(restorePlan).To(HaveLen(1))
			})

			Specify("That the single entry should have the latest timestamp", func() {
				Expect(restorePlan[0].Timestamp).To(Equal("ts0"))
			})

			Specify("That the single entry should have the current backup set FQNs", func() {
				expectedTableFQNs := []string{"public.ao1", "public.heap1"}

				Expect(restorePlan[0].ChangedTables).To(Equal(expectedTableFQNs))
			})
		})

		Context("Incremental backup", func() {
			previousRestorePlan := []backup_history.RestorePlanEntry{
				{Timestamp: "ts0", ChangedTables: []string{"public.ao1", "public.ao2"}},
				{Timestamp: "ts1", ChangedTables: []string{"public.heap1"}},
			}
			changedTables := []backup.Table{
				{Relation: backup.Relation{Schema: "public", Name: "ao1"}},
				{Relation: backup.Relation{Schema: "public", Name: "heap1"}},
			}

			Context("Incremental backup with no table drops in between", func() {
				allTables := changedTables

				restorePlan := backup.PopulateRestorePlan(changedTables, previousRestorePlan, allTables)

				It("should append 1 more entry to the previous restore plan", func() {
					Expect(restorePlan[0:2]).To(Equal(previousRestorePlan[0:2]))
					Expect(restorePlan).To(HaveLen(len(previousRestorePlan) + 1))
				})

				Specify("That the added entry should have the current backup set FQNs", func() {
					expectedTableFQNs := []string{"public.ao1", "public.heap1"}

					Expect(restorePlan[2].ChangedTables).To(Equal(expectedTableFQNs))
				})

				Specify("That the previous timestamp entries should NOT have the current backup set FQNs", func() {
					expectedTableFQNs := []string{"public.ao1", "public.heap1"}

					Expect(restorePlan[0].ChangedTables).To(Not(ContainElement(expectedTableFQNs[0])))
					Expect(restorePlan[0].ChangedTables).To(Not(ContainElement(expectedTableFQNs[1])))

					Expect(restorePlan[1].ChangedTables).To(Not(ContainElement(expectedTableFQNs[0])))
					Expect(restorePlan[1].ChangedTables).To(Not(ContainElement(expectedTableFQNs[1])))
				})

			})

			Context("A table was dropped between the last full/incremental and this incremental", func() {
				allTables := changedTables[0:1] // exclude "heap1"
				excludedTableFQN := "public.heap1"

				restorePlan := backup.PopulateRestorePlan(changedTables[0:1], previousRestorePlan, allTables)

				Specify("That the added entry should NOT have the dropped table FQN", func() {
					Expect(restorePlan[2].ChangedTables).To(Not(ContainElement(excludedTableFQN)))
				})

				Specify("That the previous timestamp entries should NOT have the dropped table FQN", func() {
					Expect(restorePlan[0].ChangedTables).To(Not(ContainElement(excludedTableFQN)))
					Expect(restorePlan[1].ChangedTables).To(Not(ContainElement(excludedTableFQN)))
				})

			})
		})

	})
	Describe("GetLatestMatchingBackupTimestamp", func() {
		var log *gbytes.Buffer
		BeforeEach(func() {
			_, _, log = testhelper.SetupTestLogger()
		})
		AfterEach(func() {
			operating.InitializeSystemFunctions()
		})
		It("fatals when trying to take an incremental backup without a full backup", func() {
			backup.SetFPInfo(backup_filepath.FilePathInfo{UserSpecifiedBackupDir: "/tmp", UserSpecifiedSegPrefix: "/test-prefix"})
			backup.SetReport(&utils.Report{})

			Expect(func() { backup.GetLatestMatchingBackupTimestamp() }).Should(Panic())
			Expect(log.Contents()).To(ContainSubstring("There was no matching previous backup found with the flags provided. Please take a full backup."))

		})
	})
})
